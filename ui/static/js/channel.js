'use strict';

// channel.js — E2E encrypted broadcast channel client.
// All crypto runs in the browser via Web Crypto API (SubtleCrypto).
// Passphrase is never stored persistently and never sent to the server.

const MSG_TYPE_TEXT = 0x01;
const MSG_TYPE_FILE = 0x02;
const VERSION_BYTE  = 0x01;
const PBKDF2_ITERS  = 600_000;

// ---------- DOM refs ----------
const channelUUID    = document.getElementById('channel-uuid').textContent.trim();
const setupPanel     = document.getElementById('setup-panel');
const channelPanel   = document.getElementById('channel-panel');
const passphraseEl   = document.getElementById('ch-passphrase');
const deviceNameEl   = document.getElementById('ch-device-name');
const connectBtn     = document.getElementById('ch-connect-btn');
const setupError     = document.getElementById('setup-error');
const statusEl       = document.getElementById('ch-status');
const messagesEl     = document.getElementById('ch-messages');
const textInput      = document.getElementById('ch-text-input');
const sendTextBtn    = document.getElementById('ch-send-text-btn');
const fileInput      = document.getElementById('ch-file-input');
const sendFileBtn    = document.getElementById('ch-send-file-btn');
const sendError      = document.getElementById('send-error');

// ---------- State ----------
let passphrase   = '';
let deviceName   = '';
const seenMsgIds = new Set();

// ---------- Connect ----------
connectBtn.addEventListener('click', async () => {
  passphrase = passphraseEl.value;
  if (!passphrase) {
    showSetupError('Passphrase is required.');
    return;
  }
  deviceName = deviceNameEl.value.trim();
  if (!deviceName) {
    deviceName = 'anon-' + Array.from(crypto.getRandomValues(new Uint8Array(2)))
      .map(b => b.toString(16).padStart(2, '0')).join('');
  }
  hideSetupError();
  connectBtn.disabled = true;
  connectBtn.textContent = 'Connecting…';

  try {
    setupPanel.hidden = true;
    channelPanel.hidden = false;
    setStatus('Connecting…');
    openSSE();
  } catch (e) {
    setupPanel.hidden = false;
    channelPanel.hidden = true;
    connectBtn.disabled = false;
    connectBtn.textContent = 'Connect';
    showSetupError('Connection failed: ' + e.message);
  }
});

// ---------- SSE ----------
function openSSE() {
  const es = new EventSource('/api/channel/' + channelUUID + '/watch');

  es.addEventListener('connected', () => {
    setStatus('Connected');
  });

  es.addEventListener('message', async (evt) => {
    let parsed;
    try { parsed = JSON.parse(evt.data); } catch { return; }
    await handleIncoming(parsed.blob, parsed.pushed_at);
  });

  es.onerror = () => {
    setStatus('Reconnecting…');
  };
}

// ---------- Send text ----------
sendTextBtn.addEventListener('click', async () => {
  const text = textInput.value.trim();
  if (!text) return;
  hideSendError();
  sendTextBtn.disabled = true;
  try {
    const payload = new TextEncoder().encode(text);
    await pushEnvelope(MSG_TYPE_TEXT, payload);
    displayMessage({ msgType: MSG_TYPE_TEXT, senderName: deviceName, payload }, new Date().toISOString());
    textInput.value = '';
  } catch (e) {
    showSendError('Send failed: ' + e.message);
  } finally {
    sendTextBtn.disabled = false;
  }
});

// ---------- Send file ----------
sendFileBtn.addEventListener('click', async () => {
  const file = fileInput.files[0];
  if (!file) { showSendError('No file selected.'); return; }
  hideSendError();
  sendFileBtn.disabled = true;
  try {
    const fileBytes = new Uint8Array(await file.arrayBuffer());
    const fnBytes   = truncateToBytes(file.name, 255); // byte-length bounded
    const fnLen     = fnBytes.length;
    const payload   = new Uint8Array(2 + fnLen + fileBytes.length);
    payload[0] = (fnLen >> 8) & 0xff;
    payload[1] = fnLen & 0xff;
    payload.set(fnBytes, 2);
    payload.set(fileBytes, 2 + fnLen);
    await pushEnvelope(MSG_TYPE_FILE, payload);
    displayMessage({ msgType: MSG_TYPE_FILE, senderName: deviceName, payload }, new Date().toISOString());
    fileInput.value = '';
  } catch (e) {
    showSendError('Send failed: ' + e.message);
  } finally {
    sendFileBtn.disabled = false;
  }
});

// ---------- Crypto helpers ----------

// Three-step PBKDF2 key derivation (required by Web Crypto API).
async function deriveKey(salt) {
  const rawPass = new TextEncoder().encode(passphrase);
  const pbkdf2Key = await crypto.subtle.importKey(
    'raw', rawPass, 'PBKDF2', false, ['deriveBits']
  );
  const rawBits = await crypto.subtle.deriveBits(
    { name: 'PBKDF2', hash: 'SHA-256', salt, iterations: PBKDF2_ITERS },
    pbkdf2Key, 256
  );
  return crypto.subtle.importKey(
    'raw', rawBits, { name: 'AES-GCM' }, false, ['encrypt', 'decrypt']
  );
}

async function encryptBlob(plaintext) {
  const salt  = crypto.getRandomValues(new Uint8Array(16));
  const nonce = crypto.getRandomValues(new Uint8Array(12));
  const aad   = new TextEncoder().encode(channelUUID); // 32 ASCII bytes
  const aesKey = await deriveKey(salt);
  const ct = await crypto.subtle.encrypt(
    { name: 'AES-GCM', iv: nonce, additionalData: aad },
    aesKey, plaintext
  );
  const blob = new Uint8Array(1 + 16 + 12 + ct.byteLength);
  blob[0] = VERSION_BYTE;
  blob.set(salt,  1);
  blob.set(nonce, 17);
  blob.set(new Uint8Array(ct), 29);
  return blob;
}

async function decryptBlob(blob) {
  if (blob[0] !== VERSION_BYTE) throw new Error('unknown version: ' + blob[0]);
  const salt  = blob.slice(1,  17);
  const nonce = blob.slice(17, 29);
  const ct    = blob.slice(29);
  const aad   = new TextEncoder().encode(channelUUID);
  const aesKey = await deriveKey(salt);
  const pt = await crypto.subtle.decrypt(
    { name: 'AES-GCM', iv: nonce, additionalData: aad },
    aesKey, ct
  );
  return new Uint8Array(pt);
}

// ---------- Utilities (defined before use) ----------

// Truncate str to at most maxBytes UTF-8 bytes without splitting a multi-byte rune.
function truncateToBytes(str, maxBytes) {
  const bytes = new TextEncoder().encode(str);
  if (bytes.length <= maxBytes) return bytes;
  let n = maxBytes;
  // UTF-8 continuation bytes are 0x80–0xBF; scan back to a start byte.
  while (n > 0 && (bytes[n] & 0xC0) === 0x80) n--;
  return bytes.slice(0, n);
}

// ---------- Envelope ----------

function buildEnvelope(msgType, senderName, payload) {
  const msgId    = crypto.getRandomValues(new Uint8Array(16));
  const nameBytes = truncateToBytes(senderName || '', 32); // byte-length bounded
  const nameLen   = nameBytes.length;
  const env = new Uint8Array(16 + 1 + 1 + nameLen + payload.length);
  let off = 0;
  env.set(msgId, off);    off += 16;
  env[off++] = msgType;
  env[off++] = nameLen;
  env.set(nameBytes, off); off += nameLen;
  env.set(payload, off);
  return { env, msgId };
}

function parseEnvelope(plain) {
  if (plain.length < 18) throw new Error('envelope too short');
  const msgId   = plain.slice(0, 16);
  const msgType = plain[16];
  if (msgType !== MSG_TYPE_TEXT && msgType !== MSG_TYPE_FILE) {
    throw new Error('unknown type: ' + msgType);
  }
  const nameLen = plain[17];
  if (nameLen > 32 || 18 + nameLen > plain.length) throw new Error('invalid name len');
  const rawName    = plain.slice(18, 18 + nameLen);
  const senderName = new TextDecoder().decode(rawName)
    .replace(/[\x00-\x1f\x7f]/g, ''); // strip control chars
  const payload = plain.slice(18 + nameLen);
  return { msgId, msgType, senderName, payload };
}

// ---------- Push ----------

async function pushEnvelope(msgType, payload) {
  const { env, msgId } = buildEnvelope(msgType, deviceName, payload);

  // Pre-register BEFORE any await so the echo cannot slip through during the
  // network round-trip. The watcher goroutine on the server can write the SSE
  // echo before the 204 response arrives, so the browser might dispatch the SSE
  // event while fetch() is still pending.
  seenMsgIds.add(msgIdKey(msgId));

  const blob = await encryptBlob(env);

  const resp = await fetch('/api/channel/' + channelUUID, {
    method:  'PUT',
    headers: { 'Content-Type': 'application/octet-stream' },
    body:    blob,
  });
  if (resp.status === 429) throw new Error('queue full — try again later');
  if (!resp.ok)           throw new Error('HTTP ' + resp.status);
}

// ---------- Receive ----------

async function handleIncoming(b64blob, pushedAt) {
  let blob;
  try {
    blob = base64ToUint8(b64blob);
  } catch { return; }

  let plain;
  try {
    plain = await decryptBlob(blob);
  } catch {
    // Decryption failure = wrong key or corrupt blob — silently discard.
    return;
  }

  let env;
  try {
    env = parseEnvelope(plain);
  } catch { return; }

  const key = msgIdKey(env.msgId);
  if (seenMsgIds.has(key)) return;
  seenMsgIds.add(key);
  // Cap the set to avoid unbounded growth.
  if (seenMsgIds.size > 200) {
    const first = seenMsgIds.values().next().value;
    seenMsgIds.delete(first);
  }

  displayMessage(env, pushedAt);
}

function displayMessage(env, pushedAt) {
  const item = document.createElement('div');
  item.className = 'ch-message';

  const header = document.createElement('div');
  header.className = 'ch-message-header';

  const ts = document.createElement('span');
  ts.className = 'ch-message-ts';
  ts.textContent = formatTS(pushedAt);
  header.appendChild(ts);

  const sep = document.createElement('span');
  sep.className = 'ch-message-sep';
  sep.textContent = '·';
  header.appendChild(sep);
  const sender = document.createElement('span');
  sender.className = 'ch-message-sender';
  sender.textContent = '(' + (env.senderName || 'anon') + ')'; // textContent — no XSS
  header.appendChild(sender);

  item.appendChild(header);
  const body = document.createElement('div');
  body.className = 'ch-message-body';

  if (env.msgType === MSG_TYPE_TEXT) {
    const pre = document.createElement('pre');
    pre.textContent = new TextDecoder().decode(env.payload); // textContent — no XSS
    body.appendChild(pre);
  } else if (env.msgType === MSG_TYPE_FILE) {
    const pl = env.payload;
    if (pl.length < 2) return;
    const fnLen = (pl[0] << 8) | pl[1];
    if (2 + fnLen > pl.length) return;
    const filename = new TextDecoder().decode(pl.slice(2, 2 + fnLen))
      .replace(/[\x00-\x1f\x7f]/g, '');
    const fileData = pl.slice(2 + fnLen);
    const url      = URL.createObjectURL(new Blob([fileData]));
    const a        = document.createElement('a');
    a.href         = url;
    a.className    = 'btn download-btn';
    // Use .download property (string), never innerHTML
    a.download     = filename;
    a.textContent  = 'Download: ' + filename; // textContent — no XSS
    // Revoke the object URL after the download starts to free memory.
    a.addEventListener('click', () => setTimeout(() => URL.revokeObjectURL(url), 60_000));
    body.appendChild(a);
  }

  item.appendChild(body);
  messagesEl.appendChild(item);
  messagesEl.scrollTop = messagesEl.scrollHeight;
}

// ---------- Utilities ----------

function base64ToUint8(b64) {
  return Uint8Array.from(atob(b64), c => c.charCodeAt(0));
}

function msgIdKey(id) {
  return Array.from(id).map(b => b.toString(16).padStart(2, '0')).join('');
}

function formatTS(iso) {
  try {
    return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
  } catch { return iso; }
}

function setStatus(msg)     { statusEl.textContent = msg; }
function showSetupError(m)  { setupError.textContent = m; setupError.hidden = false; }
function hideSetupError()   { setupError.hidden = true; }
function showSendError(m)   { sendError.textContent = m; sendError.hidden = false; }
function hideSendError()    { sendError.hidden = true; }
