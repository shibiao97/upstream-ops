"use client"

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react"
import {
  apiFetch,
  setToken,
  setUnauthorizedHandler,
} from "@/lib/api"
import type { UserRole } from "@/lib/api-types"

type AuthStatus = "loading" | "anonymous" | "authenticated"

interface AuthContextValue {
  status: AuthStatus
  username: string | null
  userID: number | null
  role: UserRole | null
  isSuperAdmin: boolean
  /** 后端关闭了鉴权（AUTH_ENABLED=false），整套 UI 当作"已登录"渲染。 */
  authDisabled: boolean
  login: (username: string, password: string) => Promise<void>
  register: (username: string, password: string, code: string) => Promise<void>
  logout: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

interface LoginResponse {
  token?: string
  expires_at?: number
  username: string
  user_id?: number
  id?: number
  role?: UserRole
  auth_disabled?: boolean
}

interface MeResponse {
  username: string
  user_id?: number
  id?: number
  role?: UserRole
  auth_disabled?: boolean
}

export function AuthProvider({ children }: { children: ReactNode }) {
  // 启动时无论有没有 token 都先 /auth/me 探测一次，因为后端可能开了"无鉴权模式"。
  const [status, setStatus] = useState<AuthStatus>("loading")
  const [username, setUsername] = useState<string | null>(null)
  const [userID, setUserID] = useState<number | null>(null)
  const [role, setRole] = useState<UserRole | null>(null)
  const [authDisabled, setAuthDisabled] = useState(false)

  useEffect(() => {
    let cancelled = false
    apiFetch<MeResponse>("/auth/me", { skipAuthErrorHandler: true })
      .then((me) => {
        if (cancelled) return
        if (me.auth_disabled) {
          // 后端关了鉴权：清掉本地任何遗留 token，避免下次开启时困惑
          setToken(null)
          setAuthDisabled(true)
          setUsername(me.username)
          setUserID(me.user_id ?? me.id ?? null)
          setRole(me.role ?? "super_admin")
          setStatus("authenticated")
          return
        }
        // 后端开启鉴权：me 成功说明现有 token 仍有效
        setUsername(me.username)
        setUserID(me.user_id ?? me.id ?? null)
        setRole(me.role ?? null)
        setStatus("authenticated")
      })
      .catch(() => {
        if (cancelled) return
        // me 失败：本地 token（如果有）已失效；显示登录页
        setToken(null)
        setUsername(null)
        setStatus("anonymous")
      })
    return () => {
      cancelled = true
    }
  }, [])

  // 注册全局 401 回调：让 apiFetch 在任何业务请求 401 时把我们打回登录页。
  // 鉴权关闭时不可能拿到 401，这里也无害。
  useEffect(() => {
    setUnauthorizedHandler(() => {
      setUsername(null)
      setUserID(null)
      setRole(null)
      setStatus("anonymous")
    })
    return () => setUnauthorizedHandler(null)
  }, [])

  const applyAuth = useCallback((res: LoginResponse) => {
    if (res.token) {
      setToken(res.token)
    }
    if (res.auth_disabled) {
      setAuthDisabled(true)
    }
    setUsername(res.username)
    setUserID(res.user_id ?? res.id ?? null)
    setRole(res.role ?? null)
    setStatus("authenticated")
  }, [])

  const login = useCallback(async (u: string, p: string) => {
    const res = await apiFetch<LoginResponse>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ username: u, password: p }),
      skipAuthErrorHandler: true,
    })
    applyAuth(res)
  }, [applyAuth])

  const register = useCallback(async (u: string, p: string, code: string) => {
    const res = await apiFetch<LoginResponse>("/auth/register", {
      method: "POST",
      body: JSON.stringify({ username: u, password: p, code }),
      skipAuthErrorHandler: true,
    })
    applyAuth(res)
  }, [applyAuth])

  const logout = useCallback(() => {
    // 鉴权关闭时 logout 按钮在 UI 上不会展示，这里仍保留兜底逻辑
    apiFetch("/auth/logout", { method: "POST" }).catch(() => {})
    setToken(null)
    setUsername(null)
    setUserID(null)
    setRole(null)
    setStatus("anonymous")
  }, [])

  const value = useMemo(
    () => ({ status, username, userID, role, isSuperAdmin: role === "super_admin", authDisabled, login, register, logout }),
    [status, username, userID, role, authDisabled, login, register, logout],
  )
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error("useAuth must be used within AuthProvider")
  }
  return ctx
}
