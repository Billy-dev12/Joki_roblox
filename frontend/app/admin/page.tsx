'use client';

import { useEffect, useRef, useState } from 'react';

interface Order {
  id: number;
  passcode: string;
  roblox_username: string;
  package_name: string;
  status: string;
  price: number;
  notes: string;
}

type StatusKey = 'pending' | 'queued' | 'in_progress' | 'completed';

function getApiUrl() {
  if (typeof window === 'undefined') return '';
  const envApi = process.env.NEXT_PUBLIC_API_URL;
  if (envApi) return envApi;
  if (window.location.port === '3000') {
    return 'http://' + window.location.hostname + ':8080';
  }
  return '';
}

const STATUS_OPTIONS: { key: StatusKey; label: string; className: string }[] = [
  { key: 'pending',     label: '⏳ Menunggu',    className: '' },
  { key: 'queued',      label: '🔄 Di Antrean',  className: 'sel-queued' },
  { key: 'in_progress', label: '⚡ Diproses',    className: 'sel-in_progress' },
  { key: 'completed',   label: '🎉 Selesai',     className: 'sel-completed' },
];

function formatRupiah(n: number) {
  return 'Rp ' + n.toLocaleString('id-ID');
}

export default function AdminPage() {
  const [phase, setPhase]     = useState<'login' | 'dashboard'>('login');
  const [password, setPassword] = useState('');
  const [loginErr, setLoginErr] = useState('');
  const [loading, setLoading]   = useState(false);

  const [orders, setOrders]         = useState<Order[]>([]);
  const [selectedPasscode, setSelectedPasscode] = useState('');
  const [selectedStatus, setSelectedStatus]     = useState<StatusKey | ''>('');
  const [notes, setNotes]           = useState('');
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [uploadProgress, setUploadProgress] = useState(0);
  const [toast, setToast]           = useState<{ msg: string; type: 'success' | 'error' } | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Auto-login check on mount
  useEffect(() => {
    async function checkAuth() {
      try {
        const apiUrl = getApiUrl();
        const res = await fetch(`${apiUrl}/api/admin/orders`, { credentials: 'include' });
        if (res.ok) {
          const data = await res.json();
          setOrders(data);
          setPhase('dashboard');
        }
      } catch { /* silently ignore and show login form */ }
    }
    checkAuth();
  }, []);

  const [adminAvatarUrl, setAdminAvatarUrl] = useState<string | null>(null);

  // Fetch selected order's Roblox avatar headshot
  useEffect(() => {
    if (!selectedPasscode || orders.length === 0) {
      setAdminAvatarUrl(null);
      return;
    }
    const o = orders.find(x => x.passcode === selectedPasscode);
    if (!o) {
      setAdminAvatarUrl(null);
      return;
    }

    async function fetchAdminAvatar(username: string) {
      setAdminAvatarUrl('loading');
      try {
        const api = getApiUrl();
        const res = await fetch(`${api}/api/roblox-avatar?username=${encodeURIComponent(username)}`);
        const data = await res.json();
        if (data.avatar_url) {
          setAdminAvatarUrl(data.avatar_url);
        } else {
          setAdminAvatarUrl('nofoto');
        }
      } catch {
        setAdminAvatarUrl('nofoto');
      }
    }

    fetchAdminAvatar(o.roblox_username);
  }, [selectedPasscode, orders]);

  function showToast(msg: string, type: 'success' | 'error' = 'success') {
    setToast({ msg, type });
    setTimeout(() => setToast(null), 3000);
  }

  async function handleLogin(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setLoginErr('');
    try {
      const res = await fetch(`${getApiUrl()}/api/admin/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ password }),
      });
      const data = await res.json();
      if (!res.ok) { setLoginErr(data.error || 'Password salah.'); setLoading(false); return; }
      setPhase('dashboard');
      fetchOrders();
    } catch {
      setLoginErr('Tidak bisa terhubung ke server.');
    }
    setLoading(false);
  }

  async function fetchOrders() {
    try {
      const res = await fetch(`${getApiUrl()}/api/admin/orders`, { credentials: 'include' });
      if (res.ok) setOrders(await res.json());
    } catch { /* ignore */ }
  }

  async function handleUpdateStatus(e: React.FormEvent) {
    e.preventDefault();
    if (!selectedPasscode || !selectedStatus) return;
    setLoading(true);
    try {
      const res = await fetch(`${getApiUrl()}/api/admin/status`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ passcode: selectedPasscode, status: selectedStatus, notes }),
      });
      const data = await res.json();
      if (!res.ok) { showToast(data.error || 'Gagal update.', 'error'); }
      else { showToast(`✅ ${data.message}`); fetchOrders(); }
    } catch { showToast('Koneksi gagal.', 'error'); }
    setLoading(false);
  }

  async function handleUpload(e: React.FormEvent) {
    e.preventDefault();
    if (!uploadFile || !selectedPasscode) return;
    setLoading(true);
    setUploadProgress(10);

    const form = new FormData();
    form.append('passcode', selectedPasscode);
    form.append('screenshot', uploadFile);

    // Simulate progress
    const interval = setInterval(() => setUploadProgress(p => Math.min(p + 15, 85)), 300);

    try {
      const res = await fetch(`${getApiUrl()}/api/admin/upload`, {
        method: 'POST',
        credentials: 'include',
        body: form,
      });
      clearInterval(interval);
      setUploadProgress(100);
      const data = await res.json();
      if (!res.ok) { showToast(data.error || 'Upload gagal.', 'error'); }
      else {
        showToast('📸 Screenshot berhasil dikirim ke pelanggan!');
        setUploadFile(null);
        if (fileInputRef.current) fileInputRef.current.value = '';
      }
    } catch {
      clearInterval(interval);
      showToast('Upload gagal. Cek koneksi.', 'error');
    }
    setTimeout(() => setUploadProgress(0), 1000);
    setLoading(false);
  }

  async function handleLogout() {
    try {
      await fetch(`${getApiUrl()}/api/admin/logout`, {
        method: 'POST',
        credentials: 'include',
      });
    } catch { /* ignore */ }
    setPhase('login');
    setPassword('');
  }

  const selectedOrder = orders.find(o => o.passcode === selectedPasscode);
  const completedOrders = orders.filter(o => o.status === 'completed');

  // ── Login Screen ──
  if (phase === 'login') return (
    <div className="page-center">
      <form className="glass login-box" onSubmit={handleLogin}>
        <span className="brand-icon">🔐</span>
        <h1 className="login-title">Admin Panel</h1>
        <p className="login-sub">Masukkan password admin untuk mengelola pesanan joki</p>
        <div className="input-wrap">
          <input
            id="admin-password"
            type="password"
            placeholder="Password admin..."
            value={password}
            onChange={e => setPassword(e.target.value)}
            autoFocus
          />
        </div>
        <button id="admin-login-btn" className="btn-primary" type="submit" disabled={loading || !password}>
          {loading ? 'Masuk...' : 'Masuk sebagai Admin →'}
        </button>
        {loginErr && <div className="error-msg">⚠️ {loginErr}</div>}
      </form>
    </div>
  );

  // ── Admin Dashboard ──
  return (
    <>
      <div className="page-top admin-page">
        {/* Header */}
        <div className="admin-header glass glass-sm" style={{ padding: '14px 20px', marginBottom: 20 }}>
          <span style={{ fontSize: 24 }}>🎮</span>
          <h1>Admin Panel</h1>
          <span className="text-muted text-xs" style={{ marginLeft: 'auto' }}>
            {orders.filter(o => o.status !== 'completed').length} pesanan aktif
          </span>
          <button
            onClick={fetchOrders}
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--purple)', fontSize: 18, marginLeft: 8 }}
            title="Refresh"
          >⟳</button>
        </div>

        {/* Select Order */}
        <div className="glass admin-card">
          <h2>Pilih Pesanan</h2>
          <label className="form-label">Username / Passcode</label>
          <select
            id="order-select"
            className="form-select"
            value={selectedPasscode}
            onChange={e => {
              setSelectedPasscode(e.target.value);
              const o = orders.find(x => x.passcode === e.target.value);
              if (o) {
                setSelectedStatus(o.status as StatusKey);
                setNotes(o.notes || '');
              }
            }}
          >
            <option value="">— Pilih pesanan —</option>
            {orders.map(o => {
              // Hanya tampilkan pesanan aktif (belum selesai), ATAU pesanan selesai yang sedang dipilih untuk di-edit
              if (o.status === 'completed' && o.passcode !== selectedPasscode) return null;
              return (
                <option key={o.id} value={o.passcode}>
                  [{o.passcode}] {o.roblox_username} · {o.package_name} {o.status === 'completed' ? '(Selesai)' : ''}
                </option>
              );
            })}
          </select>

          {selectedOrder && (
            <div 
              className="glass glass-sm" 
              style={{ 
                padding: '16px', 
                background: 'rgba(168,85,247,0.06)',
                display: 'flex',
                alignItems: 'center',
                gap: 16,
                marginTop: 12
              }}
            >
              <div className="avatar-ring" style={{ width: 60, height: 60, flexShrink: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                {adminAvatarUrl === 'loading' && <div className="spinner" style={{ width: 20, height: 20, borderWidth: 2 }} />}
                {adminAvatarUrl === 'nofoto' && <div style={{ fontSize: '0.65rem', color: 'rgba(255,255,255,0.4)', textAlign: 'center', fontWeight: 600 }}>no foto</div>}
                {adminAvatarUrl && adminAvatarUrl !== 'loading' && adminAvatarUrl !== 'nofoto' && (
                  <img src={adminAvatarUrl} alt={selectedOrder.roblox_username} style={{ width: '100%', height: '100%', borderRadius: '50%', objectFit: 'cover' }} />
                )}
              </div>
              <div style={{ flex: 1 }}>
                <h3 style={{ margin: 0, fontSize: '1.1rem', fontWeight: 600 }}>{selectedOrder.roblox_username}</h3>
                <div className="text-xs text-muted" style={{ marginTop: 4 }}>
                  Status: <strong>{{
                    pending: '⏳ Menunggu', queued: '🔄 Di Antrean',
                    in_progress: '⚡ Sedang Diproses', completed: '🎉 Selesai',
                  }[selectedOrder.status] || selectedOrder.status}</strong>
                </div>
                <div className="text-xs text-muted" style={{ marginTop: 2 }}>
                  Harga: {formatRupiah(selectedOrder.price)}
                </div>
              </div>
            </div>
          )}
        </div>

        {/* Update Status */}
        <form className="glass admin-card" onSubmit={handleUpdateStatus}>
          <h2>Update Status Joki</h2>
          <div className="status-grid">
            {STATUS_OPTIONS.map(s => (
              <button
                key={s.key}
                type="button"
                className={`status-btn ${s.className} ${selectedStatus === s.key ? 'selected' : ''}`}
                onClick={() => setSelectedStatus(s.key)}
              >
                {s.label}
              </button>
            ))}
          </div>
          <label className="form-label">Catatan (opsional)</label>
          <input
            id="notes-input"
            className="form-input"
            type="text"
            placeholder="Misal: Sedang cari Mirage Island di Third Sea..."
            value={notes}
            onChange={e => setNotes(e.target.value)}
          />
          <button
            id="update-status-btn"
            className="btn-primary"
            type="submit"
            disabled={loading || !selectedPasscode || !selectedStatus}
          >
            {loading ? 'Menyimpan...' : '⚡ Update Status & Kirim ke Pelanggan'}
          </button>
        </form>

        {/* Upload Screenshot */}
        <form className="glass admin-card" onSubmit={handleUpload}>
          <h2>Upload Screenshot Bukti</h2>
          <div className="upload-zone" onClick={() => fileInputRef.current?.click()}>
            <input
              ref={fileInputRef}
              type="file"
              accept="image/*"
              onChange={e => setUploadFile(e.target.files?.[0] || null)}
              style={{ display: 'none' }}
            />
            <div className="upload-icon">📸</div>
            {uploadFile
              ? <p><span className="upload-highlight">{uploadFile.name}</span></p>
              : <p><span className="upload-highlight">Tap di sini</span> untuk foto/pilih dari galeri HP</p>
            }
          </div>

          {uploadProgress > 0 && (
            <div className="progress-bar-wrap" style={{ marginTop: 10 }}>
              <div className="progress-bar-fill" style={{ width: `${uploadProgress}%` }} />
            </div>
          )}

          <div style={{ marginTop: 14 }}>
            <button
              id="upload-btn"
              className="btn-primary"
              type="submit"
              disabled={loading || !uploadFile || !selectedPasscode}
            >
              {loading ? 'Mengupload...' : '🚀 Upload & Kirim ke Pelanggan'}
            </button>
          </div>
        </form>

        {/* Riwayat Pesanan Selesai */}
        <div className="glass admin-card" style={{ marginTop: 24 }}>
          <h2>📜 Riwayat Pesanan Selesai</h2>
          {completedOrders.length === 0 ? (
            <p className="text-muted text-xs" style={{ padding: '8px 0' }}>Belum ada pesanan yang selesai.</p>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginTop: 12 }}>
              {completedOrders.map(o => (
                <div 
                  key={o.id} 
                  className="glass glass-sm" 
                  style={{ 
                    padding: '12px 16px', 
                    display: 'flex', 
                    justifyContent: 'space-between', 
                    alignItems: 'center',
                    background: 'rgba(16,185,129,0.03)',
                    border: '1px solid rgba(16,185,129,0.1)'
                  }}
                >
                  <div style={{ flex: 1, marginRight: 16 }}>
                    <div style={{ fontWeight: 600, fontSize: '0.95rem', display: 'flex', alignItems: 'center', gap: 8 }}>
                      <span>{o.roblox_username}</span>
                      <span className="passcode-chip" style={{ fontSize: '0.7rem', background: 'rgba(16,185,129,0.15)', color: '#10B981', padding: '2px 6px', borderRadius: '4px' }}>
                        🔑 {o.passcode}
                      </span>
                    </div>
                    <div className="text-xs text-muted" style={{ marginTop: 4 }}>
                      🏆 {o.package_name} · {formatRupiah(o.price)}
                    </div>
                    {o.notes && (
                      <div className="text-xs" style={{ marginTop: 4, color: 'rgba(255,255,255,0.5)', fontStyle: 'italic' }}>
                        "{o.notes}"
                      </div>
                    )}
                  </div>
                  <button
                    type="button"
                    className="btn-secondary"
                    style={{ width: 'auto', padding: '6px 12px', fontSize: '0.8rem', borderColor: 'rgba(16,185,129,0.3)', whiteSpace: 'nowrap' }}
                    onClick={() => {
                      setSelectedPasscode(o.passcode);
                      setSelectedStatus(o.status as StatusKey);
                      setNotes(o.notes || '');
                      window.scrollTo({ top: 0, behavior: 'smooth' });
                      showToast(`Memuat pesanan ${o.roblox_username} untuk di-edit.`);
                    }}
                  >
                    📝 Edit / Pulihkan
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Logout */}
        <div style={{ textAlign: 'center', paddingBottom: 40 }}>
          <button
            className="btn-secondary"
            style={{ maxWidth: 160 }}
            onClick={handleLogout}
          >
            ← Keluar
          </button>
        </div>
      </div>

      {/* Toast */}
      {toast && (
        <div className={`toast ${toast.type === 'success' ? 'toast-success' : 'toast-error'}`}>
          {toast.msg}
        </div>
      )}
    </>
  );
}
