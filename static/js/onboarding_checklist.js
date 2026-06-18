(function () {
  const el = document.getElementById('ob-checklist');
  if (!el) return;
  const token = () => localStorage.getItem('access_token');
  const H = () => ({ 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token() });

  // Toggle panels
  document.querySelectorAll('[data-toggle]').forEach(btn => {
    btn.addEventListener('click', () => {
      const p = document.getElementById(btn.dataset.toggle);
      if (p) p.classList.toggle('hidden');
    });
  });

  // Dismiss
  const dismiss = document.getElementById('ob-dismiss');
  if (dismiss) dismiss.addEventListener('click', async () => {
    await fetch('/api/onboarding/checklist/dismiss', { method: 'POST', headers: H() });
    el.remove();
  });

  // Invite rows
  const rows = document.getElementById('ob-invite-rows');
  function addRow() {
    const div = document.createElement('div');
    // Stack vertically in portrait so the role select never gets pushed
    // off-screen; lay out in a row only once there's room (sm+).
    div.className = 'flex flex-col sm:flex-row gap-2 mb-2 ob-invite-row';
    div.innerHTML =
      '<input type="text" placeholder="Name" class="w-full sm:flex-1 rounded-2xl border border-stone-200 px-3 py-2 ob-name"/>' +
      '<input type="email" placeholder="email" class="w-full sm:flex-1 rounded-2xl border border-stone-200 px-3 py-2 ob-email"/>' +
      '<select class="w-full sm:w-auto rounded-2xl border border-stone-200 px-2 py-2 ob-role">' +
        '<option value="parent">Parent/Guardian</option>' +
        '<option value="caregiver">Caregiver</option>' +
        '<option value="medical_provider">Medical Provider</option>' +
      '</select>';
    rows.appendChild(div);
  }
  if (rows) addRow();
  const addRowBtn = document.getElementById('ob-invite-add-row');
  if (addRowBtn) addRowBtn.addEventListener('click', addRow);

  const sendBtn = document.getElementById('ob-invite-send');
  if (sendBtn) sendBtn.addEventListener('click', async () => {
    const msg = document.getElementById('ob-invite-msg');
    const rowEls = [...document.querySelectorAll('.ob-invite-row')];
    const targets = rowEls
      .map(r => {
        const name = r.querySelector('.ob-name').value.trim();
        const sp = name.indexOf(' ');
        return {
          email: r.querySelector('.ob-email').value.trim(),
          first_name: sp === -1 ? name : name.slice(0, sp),
          last_name: sp === -1 ? '' : name.slice(sp + 1).trim(),
          role: r.querySelector('.ob-role').value,
        };
      })
      .filter(t => t.email);
    if (!targets.length) { msg.textContent = 'Enter at least one email.'; return; }
    sendBtn.disabled = true;
    let ok = 0;
    for (const t of targets) {
      const res = await fetch('/api/family/members', { method: 'POST', headers: H(),
        body: JSON.stringify({ email: t.email, first_name: t.first_name, last_name: t.last_name, role: t.role, mode: 'invite' }) });
      if (res.ok) ok++;
    }
    if (ok > 0) {
      await fetch('/api/onboarding/invite-done', { method: 'POST', headers: H() });
      msg.textContent = 'Sent ' + ok + ' invite(s)!';
    } else {
      msg.textContent = 'Could not send invites — check the addresses.';
    }
    sendBtn.disabled = false;
  });

  // Settings panel
  const tz = document.getElementById('ob-set-tz');
  if (tz) tz.value = Intl.DateTimeFormat().resolvedOptions().timeZone || 'America/Chicago';
  const saveBtn = document.getElementById('ob-set-save');
  if (saveBtn) saveBtn.addEventListener('click', async () => {
    const msg = document.getElementById('ob-set-msg');
    saveBtn.disabled = true;
    try {
      await fetch('/api/users/me/preferences', { method: 'PUT', headers: H(),
        body: JSON.stringify({ timezone: tz.value, time_format: document.getElementById('ob-set-timefmt').value }) });
      // AI consent (only enable; disclosure SHA fetched from the consent GET)
      if (document.getElementById('ob-set-ai').checked) {
        const c = await fetch('/api/users/me/narrative-consent', { headers: H() });
        if (c.ok) {
          const cd = await c.json();
          await fetch('/api/users/me/narrative-consent', { method: 'PUT', headers: H(),
            body: JSON.stringify({ enabled: true, acknowledged_sha: cd.disclosure_sha }) });
        }
      }
      await fetch('/api/onboarding/settings-done', { method: 'POST', headers: H() });
      msg.textContent = 'Saved!';
    } catch (_) { msg.textContent = 'Could not save — try again.'; }
    saveBtn.disabled = false;
  });
})();
