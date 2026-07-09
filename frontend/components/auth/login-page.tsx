"use client"

import { useEffect, useState, type FormEvent } from "react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/lib/auth-context"
import { useAppVersion } from "@/lib/queries"
import { apiFetch, type ApiError } from "@/lib/api"

export function LoginPage() {
  const { login, register } = useAuth()
  const appVersion = useAppVersion()
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [code, setCode] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [sending, setSending] = useState(false)
  const [mode, setMode] = useState<"login" | "register">("login")
  const [error, setError] = useState<string | null>(null)
  const appTitle = appVersion.data?.title?.trim() || "UpstreamOps"

  useEffect(() => {
    document.title = appTitle
  }, [appTitle])

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      if (mode === "register") {
        await register(username.trim(), password, code.trim())
      } else {
        await login(username.trim(), password, code.trim())
      }
    } catch (err) {
      const e = err as ApiError
      if (e.status === 401) {
        setError("账号或密码错误")
      } else {
        setError(e.message || "登录失败")
      }
    } finally {
      setSubmitting(false)
    }
  }

  async function sendCode() {
    setError(null)
    setSending(true)
    try {
      await apiFetch("/auth/send-code", {
        method: "POST",
        body: JSON.stringify({ username: username.trim(), action: mode }),
        skipAuthErrorHandler: true,
      })
      setError("验证码已发送，请查看邮箱")
    } catch (err) {
      const e = err as ApiError
      setError(e.message || "验证码发送失败")
    } finally {
      setSending(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4">
      <Card className="w-full max-w-sm">
        <CardHeader className="space-y-1.5">
          <CardTitle className="text-2xl">{appTitle}</CardTitle>
          <CardDescription>{mode === "register" ? "注册普通用户账号。" : "登录后台，监控渠道余额和倍率。"}</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="username">账号</Label>
              <Input
                id="username"
                name="username"
                autoComplete="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                required
                disabled={submitting}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">密码</Label>
              <Input
                id="password"
                name="password"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                disabled={submitting}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="code">邮箱验证码</Label>
              <div className="flex gap-2">
                <Input
                  id="code"
                  inputMode="numeric"
                  maxLength={6}
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  required
                  disabled={submitting}
                />
                <Button type="button" variant="outline" onClick={sendCode} disabled={submitting || sending || !username.trim()}>
                  {sending ? "发送中…" : "发送"}
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">仅支持 qq.com、163.com、gmail.com 邮箱。</p>
            </div>
            {error ? (
              <p className="text-sm text-destructive" role="alert">
                {error}
              </p>
            ) : null}
            <Button type="submit" className="w-full" disabled={submitting}>
              {submitting ? "处理中…" : mode === "register" ? "注册并登录" : "登录"}
            </Button>
            <Button
              type="button"
              variant="ghost"
              className="w-full"
              disabled={submitting}
              onClick={() => {
                setError(null)
                setCode("")
                setMode((v) => (v === "login" ? "register" : "login"))
              }}
            >
              {mode === "register" ? "已有账号，去登录" : "没有账号，注册普通用户"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
