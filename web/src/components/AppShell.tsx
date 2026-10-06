import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthProvider'
import {
  IconAccounts,
  IconAliases,
  IconICloudMailLogo,
  IconInbox,
  IconLogout,
  IconTempMail,
} from './icons'

export default function AppShell() {
  const { logout } = useAuth()
  const navigate = useNavigate()

  async function handleLogout() {
    await logout()
    navigate('/login', { replace: true })
  }

  return (
    <div className="app">
      <a className="skip-link" href="#main-content">
        跳到主要內容
      </a>

      <aside className="sidebar">
        <div className="sidebar-brand">
          <span className="brand-mark">
            <IconICloudMailLogo size={38} />
          </span>
          <span className="brand-text">
            <strong>iCloud HME Plus</strong>
            <span>Hide My Email</span>
          </span>
        </div>

        <nav className="sidebar-nav" aria-label="主要導覽">
          <NavLink
            to="/accounts"
            className={({ isActive }) => (isActive ? 'nav-item active' : 'nav-item')}
          >
            <IconAccounts size={18} />
            <span>帳號管理</span>
          </NavLink>
          <NavLink
            to="/temp"
            className={({ isActive }) => (isActive ? 'nav-item active' : 'nav-item')}
          >
            <IconTempMail size={18} />
            <span>隨機信箱</span>
          </NavLink>
          <NavLink
            to="/aliases"
            className={({ isActive }) => (isActive ? 'nav-item active' : 'nav-item')}
          >
            <IconAliases size={18} />
            <span>別名管理</span>
          </NavLink>
          <NavLink
            to="/inbox"
            className={({ isActive }) => (isActive ? 'nav-item active' : 'nav-item')}
          >
            <IconInbox size={18} />
            <span>收件匣</span>
          </NavLink>
        </nav>

        <div className="sidebar-foot">
          <button className="nav-item" onClick={() => void handleLogout()} title="登出管理台">
            <IconLogout size={18} />
            <span>登出</span>
          </button>
        </div>
      </aside>

      <main className="main" id="main-content">
        <Outlet />
      </main>
    </div>
  )
}
