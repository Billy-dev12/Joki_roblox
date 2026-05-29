'use client';

import { useEffect, useRef, useState } from 'react';

// ── Types ──────────────────────────────────────────────
interface Screenshot {
  id: number;
  order_id: number;
  filename: string;
  url: string;
  created_at: string;
}

interface OrderDetail {
  id: number;
  passcode: string;
  roblox_username: string;
  package_name: string;
  status: 'pending' | 'queued' | 'in_progress' | 'completed';
  price: number;
  notes: string;
  created_at: string;
  updated_at: string;
  screenshots: Screenshot[];
}

interface WSMessage {
  type: 'order_update' | 'new_screenshot' | 'error';
  payload: OrderDetail | Screenshot | string;
}

// ── Status Config ────────────────────────────────────
const STATUS_STEPS = [
  { key: 'pending',     label: 'Menunggu',   icon: '⏳' },
  { key: 'queued',      label: 'Di Antrean', icon: '🔄' },
  { key: 'in_progress', label: 'Diproses',   icon: '⚡' },
  { key: 'completed',   label: 'Selesai!',   icon: '🎉' },
];

const STATUS_AURA: Record<string, [string, string]> = {
  pending:     ['#F59E0B', '#F97316'],
  queued:      ['#06B6D4', '#3B82F6'],
  in_progress: ['#A855F7', '#3B82F6'],
  completed:   ['#10B981', '#06B6D4'],
};

const STATUS_LABEL: Record<string, string> = {
  pending: '⏳ Menunggu',
  queued: '🔄 Di Antrean',
  in_progress: '⚡ Sedang Diproses',
  completed: '🎉 Selesai!',
};

// ── Helpers ─────────────────────────────────────────
function getApiUrl() {
  if (typeof window === 'undefined') return '';
  const envApi = process.env.NEXT_PUBLIC_API_URL;
  if (envApi) return envApi;
  if (window.location.port === '3000') {
    return 'http://' + window.location.hostname + ':8080';
  }
  return '';
}

function formatRupiah(n: number) {
  return 'Rp ' + n.toLocaleString('id-ID');
}

function formatDate(s: string) {
  return new Date(s).toLocaleString('id-ID', {
    day: '2-digit', month: 'short', year: 'numeric',
    hour: '2-digit', minute: '2-digit',
  });
}

function setAuraColors(a: string, b: string) {
  document.documentElement.style.setProperty('--aura-a', a);
  document.documentElement.style.setProperty('--aura-b', b);
}

// ── Main Component ────────────────────────────────────
export default function Home() {
  const [phase, setPhase]       = useState<'login' | 'loading' | 'dashboard'>('login');
  const [passcode, setPasscode] = useState('');
  const [order, setOrder]       = useState<OrderDetail | null>(null);
  const [error, setError]       = useState('');
  const [lightbox, setLightbox] = useState<string | null>(null);
  const [avatarUrl, setAvatarUrl] = useState<string | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const [toast, setToast] = useState<{ msg: string; desc?: string; type: 'info' | 'screenshot' } | null>(null);

  function showLiveToast(msg: string, desc?: string, type: 'info' | 'screenshot' = 'info') {
    setToast({ msg, desc, type });
    setTimeout(() => setToast(null), 5000);
  }

  // Cleanup WebSocket on unmount
  useEffect(() => () => { wsRef.current?.close(); }, []);

  // Auto-login on mount if passcode exists in localStorage
  useEffect(() => {
    const savedCode = localStorage.getItem('joki_passcode');
    if (savedCode) {
      performLogin(savedCode, true);
    }
  }, []);

  const [notifPermission, setNotifPermission] = useState<string>('default');

  useEffect(() => {
    try {
      if (typeof window !== 'undefined' && 'Notification' in window) {
        setNotifPermission(Notification.permission);
      }
    } catch (err) {
      console.warn('Notification API access blocked or unsupported');
    }
  }, []);

  async function requestNotif() {
    try {
      if (typeof window !== 'undefined' && 'Notification' in window) {
        const p = await Notification.requestPermission();
        setNotifPermission(p);
        if (p === 'granted') {
          // Android Chrome throws Illegal Constructor for new Notification, use ServiceWorker if available
          if ('serviceWorker' in navigator) {
            navigator.serviceWorker.ready.then(sw => {
              sw.showNotification("Joki Roblox", {
                body: "Notifikasi live aktif! Kami akan mengabarimu setiap ada perubahan.",
                icon: avatarUrl && avatarUrl !== 'loading' && avatarUrl !== 'nofoto' ? avatarUrl : '/favicon.ico',
                tag: 'joki-roblox-setup'
              });
            }).catch(() => fallbackNotif("Joki Roblox", "Notifikasi live aktif!"));
          } else {
            fallbackNotif("Joki Roblox", "Notifikasi live aktif!");
          }
        }
      }
    } catch (err) {
      console.warn('Gagal meminta izin notifikasi:', err);
    }
  }

  function fallbackNotif(title: string, body: string) {
    try {
      new Notification(title, {
        body: body,
        icon: avatarUrl && avatarUrl !== 'loading' && avatarUrl !== 'nofoto' ? avatarUrl : '/favicon.ico',
      });
    } catch (e) {
      console.warn('Fallback Notification error:', e);
    }
  }

  function triggerNotification(title: string, body: string) {
    try {
      if (typeof window !== 'undefined' && 'Notification' in window && Notification.permission === 'granted') {
        if ('serviceWorker' in navigator) {
          navigator.serviceWorker.ready.then(sw => {
            sw.showNotification(title, {
              body: body,
              icon: avatarUrl && avatarUrl !== 'loading' && avatarUrl !== 'nofoto' ? avatarUrl : '/favicon.ico',
              tag: 'joki-roblox-update'
            });
          }).catch(() => fallbackNotif(title, body));
        } else {
          fallbackNotif(title, body);
        }
      }
    } catch (err) {
      console.warn('Gagal memicu notifikasi:', err);
    }
  }

  // Fetch Roblox avatar headshot
  async function fetchAvatar(username: string) {
    setAvatarUrl('loading');
    try {
      const api = getApiUrl();
      const res = await fetch(`${api}/api/roblox-avatar?username=${encodeURIComponent(username)}`);
      const data = await res.json();
      if (data.avatar_url) {
        setAvatarUrl(data.avatar_url);
      } else {
        setAvatarUrl('nofoto');
      }
    } catch { 
      setAvatarUrl('nofoto');
    }
  }

  // Connect WebSocket for live updates
  function connectWS(code: string) {
    const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
    const api = getApiUrl();
    const host = api ? new URL(api).host : window.location.host;
    const ws = new WebSocket(`${protocol}://${host}/ws?passcode=${code}`);
    wsRef.current = ws;

    ws.onmessage = (e) => {
      try {
        const msg: WSMessage = JSON.parse(e.data);
        if (msg.type === 'order_update') {
          const updated = msg.payload as OrderDetail;
          if (updated && !updated.screenshots) {
            updated.screenshots = [];
          }
          setOrder(prev => {
            if (prev && (prev.status !== updated.status || prev.notes !== updated.notes)) {
              const desc = `Status: ${STATUS_LABEL[updated.status] || updated.status}${updated.notes ? ' · ' + updated.notes : ''}`;
              triggerNotification("Update Joki: " + updated.roblox_username, desc);
              showLiveToast("⚡ Update Progress Joki!", desc, 'info');
            }
            return updated;
          });
          const [a, b] = STATUS_AURA[updated.status] || STATUS_AURA.pending;
          setAuraColors(a, b);
        } else if (msg.type === 'new_screenshot') {
          const ss = msg.payload as Screenshot;
          setOrder(prev => {
            if (prev) {
              triggerNotification(
                "Screenshot Baru! 📸",
                `Admin baru saja mengunggah bukti screenshot untuk joki kamu.`
              );
              showLiveToast("📸 Bukti Pengerjaan Baru!", "Admin baru saja mengunggah screenshot live terbaru.", 'screenshot');
              const currentSS = prev.screenshots || [];
              return { ...prev, screenshots: [...currentSS, ss] };
            }
            return prev;
          });
        }
      } catch { /* ignore malformed */ }
    };

    ws.onclose = () => {
      // Auto-reconnect after 3s
      setTimeout(() => {
        if (wsRef.current?.readyState !== WebSocket.OPEN) connectWS(code);
      }, 3000);
    };
  }

  async function performLogin(code: string, isAuto = false) {
    if (!isAuto) {
      setPhase('loading');
    }
    setError('');

    try {
      const api = getApiUrl();
      const res = await fetch(`${api}/api/order?passcode=${encodeURIComponent(code)}`);
      const data = await res.json();

      if (!res.ok) {
        setError(data.error || 'Passcode tidak ditemukan.');
        setPhase('login');
        localStorage.removeItem('joki_passcode');
        return;
      }

      if (data && !data.screenshots) {
        data.screenshots = [];
      }

      setPasscode(code);
      setOrder(data);
      const [a, b] = STATUS_AURA[data.status] || STATUS_AURA.pending;
      setAuraColors(a, b);
      setPhase('dashboard');
      fetchAvatar(data.roblox_username);
      connectWS(code);
    } catch {
      if (!isAuto) {
        setError('Tidak bisa terhubung ke server. Coba lagi.');
        setPhase('login');
      } else {
        setPhase('login');
      }
    }
  }

  async function handleLogin(e: React.FormEvent) {
    e.preventDefault();
    const code = passcode.trim().toUpperCase();
    if (!code) return;

    localStorage.setItem('joki_passcode', code);
    await performLogin(code);
  }

  function getStepState(stepKey: string) {
    if (!order) return 'idle';
    const order_idx  = STATUS_STEPS.findIndex(s => s.key === order.status);
    const step_idx   = STATUS_STEPS.findIndex(s => s.key === stepKey);
    if (step_idx < order_idx) return 'done';
    if (step_idx === order_idx) return 'active';
    return 'idle';
  }

  // ── RENDER: Login ──
  if (phase === 'login') return (
    <div className="page-center">
      <form className="glass login-box" onSubmit={handleLogin}>
        <span className="brand-icon">🎮</span>
        <h1 className="login-title">Joki Roblox</h1>
        <p className="login-sub">Masukkan passcode dari admin untuk melihat status jokiroblox kamu</p>
        <div className="input-wrap">
          <input
            id="passcode-input"
            type="text"
            placeholder="BJ-XXXX"
            value={passcode}
            onChange={e => setPasscode(e.target.value.toUpperCase())}
            maxLength={7}
            autoFocus
          />
        </div>
        <button id="login-btn" className="btn-primary" type="submit" disabled={passcode.length < 4}>
          Cek Status Joki →
        </button>
        {error && <div className="error-msg">⚠️ {error}</div>}
        <p className="text-muted text-xs mt-4">
          Passcode didapat dari admin setelah kamu order joki
        </p>
      </form>
    </div>
  );

  // ── RENDER: Loading ──
  if (phase === 'loading') return (
    <div className="page-center">
      <div style={{ textAlign: 'center' }}>
        <div className="spinner" />
        <p className="text-muted" style={{ marginTop: 16, fontSize: '0.9rem' }}>Mengambil data order...</p>
      </div>
    </div>
  );

  // ── RENDER: Dashboard ──
  if (!order) return null;

  const screenshots = order.screenshots || [];

  return (
    <>
      <div className="page-top">
        {/* Notification Request Banner */}
        {typeof window !== 'undefined' && 'Notification' in window && notifPermission === 'default' && (
          <div 
            className="glass" 
            style={{ 
              padding: '12px 20px', 
              marginBottom: 16, 
              display: 'flex', 
              justifyContent: 'space-between', 
              alignItems: 'center', 
              background: 'rgba(168,85,247,0.08)', 
              borderColor: 'rgba(168,85,247,0.2)',
              borderRadius: 16
            }}
          >
            <span style={{ fontSize: '0.85rem', fontWeight: 500 }}>🔔 Nyalakan notifikasi live untuk mendapatkan update langsung di HP/browser kamu!</span>
            <button 
              className="btn-primary" 
              style={{ width: 'auto', padding: '6px 16px', fontSize: '0.8rem', margin: 0, whiteSpace: 'nowrap', marginLeft: 16 }}
              onClick={requestNotif}
            >
              Aktifkan
            </button>
          </div>
        )}

        {/* Header */}
        <div className="glass dash-header">
          <div className="avatar-ring" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            {avatarUrl === 'loading' && <div className="spinner" style={{ width: 30, height: 30, borderWidth: 3 }} />}
            {avatarUrl === 'nofoto' && <div style={{ fontSize: '0.7rem', color: 'rgba(255,255,255,0.4)', fontWeight: 600 }}>no foto</div>}
            {avatarUrl && avatarUrl !== 'loading' && avatarUrl !== 'nofoto' && (
              <img src={avatarUrl} alt={order.roblox_username} />
            )}
          </div>
          <div className="user-info">
            <h2>{order.roblox_username}</h2>
            <span className="package-badge">🏆 {order.package_name}</span>
          </div>
          <div className="price-tag">
            <div className="label">Harga</div>
            <div className="amount">{formatRupiah(order.price)}</div>
          </div>
        </div>

        {/* Status Stepper */}
        <div className="glass stepper-card">
          <h3>Progress Joki</h3>
          <div className="stepper">
            {STATUS_STEPS.map(step => {
              const state = getStepState(step.key);
              return (
                <div className="step" key={step.key}>
                  <div className={`step-dot ${state}`}>{step.icon}</div>
                  <span className={`step-label ${state}`}>{step.label}</span>
                </div>
              );
            })}
          </div>
        </div>

        {/* Notes from admin */}
        <div className="glass notes-card">
          <div className="notes-label">
            Status Terkini
            {order.status === 'in_progress' && (
              <span className="live-badge">
                <span className="live-dot" />LIVE
              </span>
            )}
          </div>
          <div className="notes-text">
            {order.notes || STATUS_LABEL[order.status] || '—'}
          </div>
          <div className="notes-time">
            Update terakhir: {formatDate(order.updated_at)}
            <span style={{ marginLeft: 12 }} className="passcode-chip">
              🔑 {order.passcode}
            </span>
          </div>
        </div>

        {/* Screenshot Gallery */}
        <div className="glass gallery-card">
          <div className="gallery-header">
            <h3>Screenshot Bukti Joki</h3>
            {screenshots.length > 0 && (
              <span className="live-badge" style={{ marginLeft: 8 }}>
                <span className="live-dot" />{screenshots.length} foto
              </span>
            )}
          </div>
          {screenshots.length === 0
            ? <div className="gallery-empty">📸 Belum ada screenshot. Foto akan muncul saat admin sedang mengerjakan.</div>
            : (
              <div className="gallery-grid">
                {screenshots.map(ss => (
                  <div
                    key={ss.id}
                    className="gallery-item"
                    onClick={() => setLightbox(`${getApiUrl()}${ss.url}`)}
                    title="Klik untuk perbesar"
                  >
                    <img src={`${getApiUrl()}${ss.url}`} alt={`Screenshot ${ss.id}`} />
                  </div>
                ))}
              </div>
            )
          }
        </div>

        {/* Back button */}
        <div style={{ textAlign: 'center', paddingBottom: 32 }}>
          <button
            className="btn-secondary"
            style={{ maxWidth: 200 }}
            onClick={() => {
              wsRef.current?.close();
              setOrder(null);
              setPasscode('');
              setAvatarUrl(null);
              setAuraColors('#A855F7', '#3B82F6');
              setPhase('login');
              localStorage.removeItem('joki_passcode');
            }}
          >
            ← Kembali
          </button>
        </div>
      </div>

      {/* Lightbox */}
      {lightbox && (
        <div className="lightbox-overlay" onClick={() => setLightbox(null)}>
          <button className="lightbox-close" onClick={() => setLightbox(null)}>✕</button>
          <img
            className="lightbox-img"
            src={lightbox}
            alt="Preview screenshot"
            onClick={e => e.stopPropagation()}
          />
        </div>
      )}

      {/* Live In-App Toast Notification */}
      {toast && (
        <div 
          className="glass" 
          style={{ 
            position: 'fixed', 
            bottom: 24, 
            right: 24, 
            left: 24,
            maxWidth: 400,
            margin: '0 auto',
            padding: '16px 20px', 
            background: 'rgba(15,12,28,0.85)',
            backdropFilter: 'blur(16px)',
            border: '1px solid rgba(168,85,247,0.3)',
            boxShadow: '0 10px 30px rgba(168,85,247,0.2), inset 0 0 15px rgba(168,85,247,0.1)',
            borderRadius: 16,
            zIndex: 9999,
            display: 'flex',
            alignItems: 'center',
            gap: 14,
            animation: 'slideUp 0.3s cubic-bezier(0.16, 1, 0.3, 1)'
          }}
        >
          <span style={{ fontSize: '1.8rem' }}>{toast.type === 'screenshot' ? '📸' : '🔔'}</span>
          <div style={{ flex: 1 }}>
            <div style={{ fontWeight: 600, fontSize: '0.95rem', color: '#fff' }}>{toast.msg}</div>
            {toast.desc && <div style={{ fontSize: '0.8rem', color: 'rgba(255,255,255,0.7)', marginTop: 4 }}>{toast.desc}</div>}
          </div>
          <button 
            onClick={() => setToast(null)} 
            style={{ background: 'none', border: 'none', color: 'rgba(255,255,255,0.4)', cursor: 'pointer', fontSize: '1rem', padding: 4 }}
          >✕</button>
        </div>
      )}

      {/* CSS Animation keyframes for live toast */}
      <style dangerouslySetInnerHTML={{__html: `
        @keyframes slideUp {
          from { transform: translateY(100px); opacity: 0; }
          to { transform: translateY(0); opacity: 1; }
        }
      `}} />
    </>
  );
}
