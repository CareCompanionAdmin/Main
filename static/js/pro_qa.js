(function () {
  // Markdown preview toggle (Info page)
  document.addEventListener('click', function (e) {
    const btn = e.target.closest('[data-pq-tab]');
    if (!btn) return;
    const tab = btn.dataset.pqTab;
    document.querySelectorAll('[data-pq-pane]').forEach(p => {
      p.classList.toggle('hidden', p.dataset.pqPane !== tab);
    });
    document.querySelectorAll('.pq-tab-btn').forEach(b => {
      b.classList.toggle('bg-gray-200', b.dataset.pqTab === tab);
    });
  });

  // Attachment upload (issue detail page)
  const form = document.getElementById('pq-upload-form');
  if (!form) return;
  form.addEventListener('submit', async function (e) {
    e.preventDefault();
    const issueId = form.dataset.issueId;
    const status = document.getElementById('pq-upload-status');
    const fd = new FormData(form);
    status.textContent = 'Uploading…';
    try {
      const res = await fetch(`/admin/pro-qa/issues/${issueId}/attach`, {
        method: 'POST',
        body: fd,
        credentials: 'same-origin',
      });
      if (!res.ok) throw new Error(await res.text());
      status.textContent = 'Uploaded. Reloading…';
      setTimeout(() => location.reload(), 400);
    } catch (err) {
      status.textContent = 'Upload failed: ' + err.message;
    }
  });
})();
