(function () {
  const token = () => localStorage.getItem('access_token');
  const authHeaders = () => ({ 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token() });

  const CONDITIONS = ['Autism Spectrum Disorder', 'ADHD', 'Anxiety', 'Epilepsy/Seizures', 'Sensory Processing Disorder', 'Speech/Language Delay'];
  const selected = new Set();
  // Once the child is created we must not create it again on a retry — if the
  // follow-up /onboarding/complete call fails, re-clicking Finish should only
  // re-issue completion, not POST a second child.
  let childCreated = false;

  // Build the step order based on the user's situation.
  let steps;
  if (window.OB.invitedMember) {
    steps = ['welcome', 'settings'];           // trimmed path
  } else if (window.OB.hasFamily) {
    steps = ['welcome', 'child'];
  } else {
    steps = ['welcome', 'family', 'child'];
  }
  let idx = 0;

  function show(i) {
    idx = i;
    document.querySelectorAll('[data-step]').forEach(s => { s.hidden = (s.dataset.step !== steps[i]); });
    document.getElementById('ob-progress').style.width = Math.round((i / (steps.length - 1)) * 100) + '%';
    if (steps[i] === 'settings') {
      const tz = Intl.DateTimeFormat().resolvedOptions().timeZone || 'America/Chicago';
      document.getElementById('ob-tz').value = tz;
    }
  }
  function next() { if (idx < steps.length - 1) show(idx + 1); }
  function back() { if (idx > 0) show(idx - 1); }
  function err(sectionStep, msg) {
    const sec = document.querySelector('[data-step="' + sectionStep + '"]');
    const p = sec.querySelector('[data-error]');
    if (p) { p.textContent = msg; p.classList.remove('hidden'); }
  }

  // Condition chips
  function renderChips() {
    const wrap = document.getElementById('ob-chips');
    wrap.innerHTML = '';
    const all = CONDITIONS.concat([...selected].filter(c => !CONDITIONS.includes(c)));
    all.forEach(c => {
      const on = selected.has(c);
      const b = document.createElement('button');
      b.type = 'button';
      b.textContent = c + (on ? '  ✕' : '');
      b.className = 'rounded-full px-3 py-1 text-sm ' + (on ? 'bg-orange-500 text-white' : 'bg-stone-100 text-stone-700');
      b.onclick = () => { on ? selected.delete(c) : selected.add(c); renderChips(); };
      wrap.appendChild(b);
    });
  }

  document.addEventListener('click', async (e) => {
    if (e.target.matches('[data-next]')) return next();
    if (e.target.matches('[data-back]')) return back();

    if (e.target.matches('[data-save-family]')) {
      const name = document.getElementById('ob-family-name').value.trim();
      if (!name) return err('family', 'Please enter a family name.');
      e.target.disabled = true;
      try {
        const res = await fetch('/api/families', { method: 'POST', headers: authHeaders(), body: JSON.stringify({ name }) });
        const data = await res.json();
        if (!res.ok) { e.target.disabled = false; return err('family', data.message || 'Could not create family.'); }
        // Switch context so the JWT carries the new family_id.
        const sw = await fetch('/api/auth/switch-family', { method: 'POST', headers: authHeaders(), credentials: 'same-origin', body: JSON.stringify({ family_id: data.id }) });
        if (sw.ok) { const sd = await sw.json(); if (sd.access_token) localStorage.setItem('access_token', sd.access_token); }
        next();
      } catch (_) { e.target.disabled = false; err('family', 'Network error — please try again.'); }
    }

    if (e.target.matches('[data-finish]')) {
      const first = document.getElementById('ob-child-first').value.trim();
      const dob = document.getElementById('ob-child-dob').value;
      if (!first) return err('child', 'Please enter a first name.');
      if (!dob) return err('child', 'Please enter a date of birth.');
      const body = {
        first_name: first,
        date_of_birth: new Date(dob).toISOString(),
        gender: document.getElementById('ob-child-gender').value || undefined,
        conditions: [...selected]
      };
      e.target.disabled = true;
      try {
        // Create the child only once. A previous click may have created it but
        // failed on the completion step below; in that case skip straight to
        // re-issuing completion so we never POST a duplicate child.
        if (!childCreated) {
          const res = await fetch('/api/children', { method: 'POST', headers: authHeaders(), body: JSON.stringify(body) });
          const data = await res.json();
          if (!res.ok) { e.target.disabled = false; return err('child', data.message || 'Could not save child.'); }
          childCreated = true;
        }
        // Only navigate once onboarding is actually marked complete — otherwise
        // the dashboard gate bounces the user back here and (without the guard
        // above) they would create a second child.
        const cr = await fetch('/api/onboarding/complete', { method: 'POST', headers: authHeaders() });
        if (!cr.ok) { e.target.disabled = false; return err('child', 'Almost done — please tap Finish again.'); }
        window.location.href = '/dashboard';
      } catch (_) { e.target.disabled = false; err('child', childCreated ? 'Almost done — please tap Finish again.' : 'Network error — please try again.'); }
    }

    if (e.target.matches('[data-finish-invited]')) {
      const tz = document.getElementById('ob-tz').value;
      e.target.disabled = true;
      try {
        await fetch('/api/users/me/preferences', { method: 'PUT', headers: authHeaders(), body: JSON.stringify({ timezone: tz }) });
        await fetch('/api/onboarding/complete', { method: 'POST', headers: authHeaders() });
        window.location.href = '/dashboard';
      } catch (_) { e.target.disabled = false; }
    }

    if (e.target.matches('#ob-chip-add')) {
      const v = document.getElementById('ob-chip-custom').value.trim();
      if (v) { selected.add(v); document.getElementById('ob-chip-custom').value = ''; renderChips(); }
    }
  });

  renderChips();
  show(0);
})();
