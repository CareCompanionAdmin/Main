package service

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AppStoreConnectService talks to Apple's App Store Connect API for beta-tester
// management. The team-key JWT is regenerated on demand (Apple's max lifetime
// is 20 minutes); the External Beta Group ID is resolved once and cached.
type AppStoreConnectService struct {
	issuerID      string
	keyID         string
	privateKey    *ecdsa.PrivateKey
	betaGroupName string

	httpClient *http.Client

	// cached group ID for betaGroupName, looked up once on first use.
	groupOnce sync.Once
	groupID   string
	groupErr  error
}

// NewAppStoreConnectService constructs the service. If issuerID, keyID, or
// keyPath is empty, returns nil — callers should treat that as "ASC integration
// disabled" and fall back to manual workflow (e.g. log the email and let an
// admin add the tester by hand).
func NewAppStoreConnectService(issuerID, keyID, keyPath, betaGroupName string) (*AppStoreConnectService, error) {
	if issuerID == "" || keyID == "" || keyPath == "" {
		return nil, nil
	}
	if betaGroupName == "" {
		return nil, errors.New("appstoreconnect: ASC_BETA_GROUP_NAME is required")
	}

	pemBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("appstoreconnect: read key file %q: %w", keyPath, err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("appstoreconnect: %q does not contain a PEM block", keyPath)
	}
	pkcs8, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("appstoreconnect: parse PKCS8 key: %w", err)
	}
	ecKey, ok := pkcs8.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("appstoreconnect: private key is not ECDSA (App Store Connect requires ES256)")
	}

	return &AppStoreConnectService{
		issuerID:      issuerID,
		keyID:         keyID,
		privateKey:    ecKey,
		betaGroupName: betaGroupName,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// makeJWT mints a short-lived ES256 token for the App Store Connect API.
// Apple caps lifetime at 20 minutes; we use 15 to leave margin.
func (s *AppStoreConnectService) makeJWT() (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": s.issuerID,
		"iat": now.Unix(),
		"exp": now.Add(15 * time.Minute).Unix(),
		"aud": "appstoreconnect-v1",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["kid"] = s.keyID
	tok.Header["typ"] = "JWT"
	return tok.SignedString(s.privateKey)
}

// do executes a request against the ASC API with a fresh JWT.
func (s *AppStoreConnectService) do(ctx context.Context, method, path string, query url.Values, body interface{}) ([]byte, int, error) {
	jwtStr, err := s.makeJWT()
	if err != nil {
		return nil, 0, fmt.Errorf("appstoreconnect: sign JWT: %w", err)
	}

	u := "https://api.appstoreconnect.apple.com" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("appstoreconnect: marshal body: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+jwtStr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("appstoreconnect: HTTP: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("appstoreconnect: read body: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// resolveGroupID looks up the External Beta Group ID by name, once. Cached for
// the life of the service. ASC group names are unique within a team.
func (s *AppStoreConnectService) resolveGroupID(ctx context.Context) (string, error) {
	s.groupOnce.Do(func() {
		q := url.Values{}
		q.Set("filter[name]", s.betaGroupName)
		q.Set("limit", "200")

		body, status, err := s.do(ctx, http.MethodGet, "/v1/betaGroups", q, nil)
		if err != nil {
			s.groupErr = err
			return
		}
		if status != http.StatusOK {
			s.groupErr = fmt.Errorf("appstoreconnect: list betaGroups returned %d: %s", status, string(body))
			return
		}
		var parsed struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					Name string `json:"name"`
				} `json:"attributes"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			s.groupErr = fmt.Errorf("appstoreconnect: parse betaGroups: %w", err)
			return
		}
		// API filter is contains-style; pin to exact match.
		for _, g := range parsed.Data {
			if g.Attributes.Name == s.betaGroupName {
				s.groupID = g.ID
				return
			}
		}
		s.groupErr = fmt.Errorf("appstoreconnect: no Beta Group named %q (found %d candidates)", s.betaGroupName, len(parsed.Data))
	})
	return s.groupID, s.groupErr
}

// AddExternalTester registers an email as an external tester and adds them to
// the configured Beta Group. firstName/lastName are required by Apple's API.
// Returns nil on success; an error wrapping ASC's response body on failure.
func (s *AppStoreConnectService) AddExternalTester(ctx context.Context, email, firstName, lastName string) error {
	groupID, err := s.resolveGroupID(ctx)
	if err != nil {
		return err
	}

	body := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "betaTesters",
			"attributes": map[string]interface{}{
				"email":     email,
				"firstName": firstName,
				"lastName":  lastName,
			},
			"relationships": map[string]interface{}{
				"betaGroups": map[string]interface{}{
					"data": []map[string]string{
						{"type": "betaGroups", "id": groupID},
					},
				},
			},
		},
	}

	respBody, status, err := s.do(ctx, http.MethodPost, "/v1/betaTesters", nil, body)
	if err != nil {
		return err
	}
	// 201 Created is the success path. 409 Conflict is "already a tester" —
	// we treat that as success (idempotent invite).
	if status == http.StatusCreated || status == http.StatusConflict {
		return nil
	}
	return fmt.Errorf("appstoreconnect: create betaTester returned %d: %s", status, string(respBody))
}

// IsConfigured reports whether the service is wired up. Callers can short-
// circuit the auto-add step and fall back to a manual workflow if false.
func (s *AppStoreConnectService) IsConfigured() bool {
	return s != nil && s.privateKey != nil
}
