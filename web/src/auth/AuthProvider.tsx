import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { request, registerUnauthorizedHandler, setCSRFToken } from '../api/client'
import type { LoginResult } from '../api/types'

type AuthStatus = 'checking' | 'anonymous' | 'authenticated'

interface AuthContextValue {
  status: AuthStatus
  login: (password: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth 必須在 AuthProvider 內使用')
  }
  return ctx
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>('checking')
  const checkRef = useRef(false)

  useEffect(() => {
    if (checkRef.current) return
    checkRef.current = true
    let cancelled = false
    request<LoginResult>('/api/auth/session', undefined, () => {
      // 工作階段探測本身的 401 不算「逾時」，交由下方狀態判斷處理
    })
      .then((data) => {
        if (cancelled) return
        setCSRFToken(data.csrf_token)
        setStatus('authenticated')
      })
      .catch(() => {
        if (cancelled) return
        setStatus('anonymous')
      })
    return () => {
      cancelled = true
    }
  }, [])

  const logout = useCallback(async () => {
    try {
      await request('/api/auth/logout', { method: 'POST' })
    } catch {
      // 無論請求結果如何都清空本機狀態
    }
    setCSRFToken(null)
    setStatus('anonymous')
  }, [])

  const handleUnauthorized = useCallback(() => {
    setCSRFToken(null)
    setStatus('anonymous')
  }, [])

  useEffect(() => {
    registerUnauthorizedHandler(handleUnauthorized)
    return () => registerUnauthorizedHandler(null)
  }, [handleUnauthorized])

  const login = useCallback(async (password: string) => {
    const data = await request<LoginResult>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ password }),
    })
    setCSRFToken(data.csrf_token)
    setStatus('authenticated')
  }, [])

  const value = useMemo(() => ({ status, login, logout }), [status, login, logout])

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
