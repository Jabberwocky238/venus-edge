import { NavLink, Navigate, Outlet, Route, Routes } from 'react-router-dom'
import DNSPage from './pages/DNS.tsx'
import HTTPPage from './pages/http.tsx'
import TLSPage from './pages/tls.tsx'

function AppLayout() {
  const navClassName = ({ isActive }: { isActive: boolean }) =>
    `rounded-full px-4 py-2 text-sm font-semibold transition ${
      isActive
        ? 'bg-slate-950 text-white'
        : 'bg-white/80 text-slate-700 ring-1 ring-inset ring-slate-200 hover:bg-slate-100'
    }`

  return (
    <div className="mx-auto min-h-screen w-full max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
      <header className="overflow-hidden rounded-[28px] border border-slate-200/80 bg-white/85 shadow-[0_24px_80px_-32px_rgba(15,23,42,0.35)] backdrop-blur">
        <div className="grid gap-6 bg-[radial-gradient(circle_at_top_left,_rgba(14,165,233,0.18),_transparent_34%),linear-gradient(135deg,_rgba(255,255,255,0.96),_rgba(248,250,252,0.92))] px-6 py-8 sm:px-8 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end">
          <div className="space-y-4">
            <p className="text-xs font-semibold uppercase tracking-[0.35em] text-cyan-700">
              Venus Edge
            </p>
            <div className="space-y-3">
              <h1 className="text-4xl font-semibold tracking-tight text-slate-950 sm:text-5xl">
                Builder Console
              </h1>
              <p className="max-w-3xl text-sm leading-7 text-slate-600 sm:text-base">
                Query the DNS, TLS, and HTTP resources bound to a hostname, edit
                the full builder payload, and push the complete resource set back
                to master for publish.
              </p>
            </div>
          </div>
          <nav className="flex flex-wrap gap-2">
            <NavLink to="/dns" className={navClassName}>
              DNS
            </NavLink>
            <NavLink to="/tls" className={navClassName}>
              TLS
            </NavLink>
            <NavLink to="/http" className={navClassName}>
              HTTP
            </NavLink>
          </nav>
        </div>
      </header>

      <main className="mt-6">
        <Outlet />
      </main>
    </div>
  )
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<AppLayout />}>
        <Route index element={<Navigate replace to="/dns" />} />
        <Route path="dns" element={<DNSPage />} />
        <Route path="tls" element={<TLSPage />} />
        <Route path="http" element={<HTTPPage />} />
        <Route path="*" element={<Navigate replace to="/dns" />} />
      </Route>
    </Routes>
  )
}
