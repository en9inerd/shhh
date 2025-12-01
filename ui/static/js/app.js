const copyToClipboard = (text) => {
  navigator.clipboard.writeText(text).then(() => alert('Copied to clipboard!'))
    .catch(err => console.error('Failed to copy:', err));
};

const getMimeType = (filename) => {
  const mimeTypes = {
    txt: 'text/plain', pdf: 'application/pdf',
    doc: 'application/msword', docx: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    xls: 'application/vnd.ms-excel', xlsx: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg', gif: 'image/gif', svg: 'image/svg+xml',
    zip: 'application/zip', rar: 'application/x-rar-compressed', tar: 'application/x-tar', gz: 'application/gzip',
    json: 'application/json', xml: 'application/xml', html: 'text/html', css: 'text/css', js: 'text/javascript', csv: 'text/csv',
  };
  return mimeTypes[filename?.toLowerCase().split('.').pop()] || 'application/octet-stream';
};

const downloadFile = (fileDataB64, filename) => {
  try {
    const bytes = Uint8Array.from(atob(fileDataB64), c => c.charCodeAt(0));
    const blob = new Blob([bytes], { type: getMimeType(filename) });
    const url = URL.createObjectURL(blob);
    const a = Object.assign(document.createElement('a'), { href: url, download: filename || 'download', style: 'display:none' });
    document.body.appendChild(a).click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  } catch (err) {
    console.error('Download error:', err);
    alert('Failed to download file: ' + err.message);
  }
};

document.addEventListener('click', (e) => {
  if (e.target.matches('.copy-btn[data-copy-text]')) {
    copyToClipboard(e.target.getAttribute('data-copy-text'));
    e.preventDefault();
  } else if (e.target.matches('.copy-btn[data-copy-selector]')) {
    const el = document.querySelector(e.target.getAttribute('data-copy-selector'));
    if (el) copyToClipboard(el.textContent);
    e.preventDefault();
  } else {
    const btn = e.target.closest('.download-btn[data-file-data]');
    if (btn) {
      e.preventDefault();
      const data = btn.getAttribute('data-file-data');
      if (data) downloadFile(data, btn.getAttribute('data-filename'));
      else alert('File data not available');
    }
  }
});

document.addEventListener('change', (e) => {
  if (e.target.matches('select[name="exp_unit"]')) {
    const container = e.target.nextElementSibling;
    if (container?.classList.contains('custom-exp')) {
      container.style.display = e.target.value === 'custom' ? 'block' : 'none';
    }
  }
});
