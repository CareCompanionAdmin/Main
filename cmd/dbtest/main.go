package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5"
)

func main() {
	config, err := pgx.ParseConfig("")
	if err != nil {
		log.Fatalf("ParseConfig failed: %v", err)
	}

	config.Host = "172.28.0.10"
	config.Port = 5432
	config.Database = "carecompanion"
	config.User = "carecomp_app"
	config.Password = "CareCompApp2025\\!"  // Single backslash
	
	log.Printf("Password: %q", config.Password)
	for i, c := range config.Password {
		log.Printf("  [%d] %c = %d", i, c, c)
	}
	
	conn, err := pgx.ConnectConfig(context.Background(), config)
	if err != nil {
		log.Fatalf("Connect failed: %v", err)
	}
	defer conn.Close(context.Background())
	
	var result int
	err = conn.QueryRow(context.Background(), "SELECT 1").Scan(&result)
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}
	
	log.Printf("SUCCESS! Result: %d", result)
}
