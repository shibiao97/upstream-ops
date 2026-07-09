"use client"

import { useEffect, useMemo, useState, type FormEvent } from "react"
import { ChevronDown, ChevronRight, RefreshCw, Settings, Server } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Switch } from "@/components/ui/switch"
import { apiFetch } from "@/lib/api"
import type { RelayAccount, RelayConfig, RelaySummary, RelayUserUsage, RelayUsersPage } from "@/lib/api-types"
import { money, relativeTime } from "@/lib/format"
import { useTriggerRefresh } from "@/lib/refresh-context"
import { useRelayConfig, useRelaySummary } from "@/lib/queries"
import { cn } from "@/lib/utils"

interface MultiplierFormRow {
  account_id: number
  name: string
  multiplier: string
}

interface RelayForm {
  name: string
  site_url: string
  admin_email: string
  password: string
  enabled: boolean
  account_multipliers: MultiplierFormRow[]
}

const PAGE_SIZE = 20

function todayLocal() {
  const d = new Date()
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, "0")
  const day = String(d.getDate()).padStart(2, "0")
  return `${y}-${m}-${day}`
}

function initialForm(config?: RelayConfig | null): RelayForm {
  return {
    name: config?.name ?? "自有中转站",
    site_url: config?.site_url ?? "",
    admin_email: config?.admin_email ?? "",
    password: "",
    enabled: config?.enabled ?? true,
    account_multipliers: (config?.account_multipliers ?? []).map((m) => ({
      account_id: m.account_id,
      name: m.name,
      multiplier: String(m.multiplier || 1),
    })),
  }
}

function bodyFromForm(form: RelayForm) {
  return {
    name: form.name,
    site_url: form.site_url,
    admin_email: form.admin_email,
    password: form.password,
    enabled: form.enabled,
    account_multipliers: form.account_multipliers.map((m) => ({
      account_id: m.account_id,
      name: m.name,
      multiplier: Number(m.multiplier) > 0 ? Number(m.multiplier) : 1,
    })),
  }
}

function mergeAccounts(rows: MultiplierFormRow[], accounts: RelayAccount[]) {
  const existing = new Map(rows.map((r) => [r.account_id, r]))
  return accounts.map((account) => {
    const old = existing.get(account.id)
    return {
      account_id: account.id,
      name: old?.name || account.name || `账号 #${account.id}`,
      multiplier: old?.multiplier || String(account.rate_multiplier || 1),
    }
  })
}

export function RelayUsageCard() {
  const [configOpen, setConfigOpen] = useState(false)
  const [usersOpen, setUsersOpen] = useState(false)
  const config = useRelayConfig()
  const summary = useRelaySummary()

  const data = summary.data
  const configured = data?.configured || config.data?.configured

  return (
    <>
      <Card className="min-h-0 overflow-hidden border border-border shadow-none lg:h-80">
        <CardHeader className="flex shrink-0 flex-row items-center justify-between px-4 pb-2 sm:px-6">
          <CardTitle className="flex items-center gap-2 text-base font-semibold">
            <Server className="size-4 text-brand" />
            {"自有中转站"}
          </CardTitle>
          <Button variant="ghost" size="sm" className="h-7 px-2" onClick={() => setConfigOpen(true)}>
            <Settings className="size-3.5" />
          </Button>
        </CardHeader>
        <CardContent className="flex h-full min-h-0 flex-col px-4 pb-4 sm:px-6">
          {!configured ? (
            <div className="flex flex-1 flex-col items-center justify-center rounded-lg border border-dashed border-border px-4 text-center">
              <p className="text-sm font-medium text-foreground">{"还没有配置 Sub2API 管理端"}</p>
              <Button size="sm" className="mt-3" onClick={() => setConfigOpen(true)}>
                {"配置中转站"}
              </Button>
            </div>
          ) : summary.loading ? (
            <div className="flex flex-1 items-center justify-center text-xs text-muted-foreground">{"加载中…"}</div>
          ) : (
            <div className="flex min-h-0 flex-1 flex-col">
              <div className="grid grid-cols-2 gap-3">
                <Metric label="今日用户消费" value={money(data?.actual_cost)} tone="brand" />
                <Metric label="我的成本" value={money(data?.cost)} tone="warning" />
              </div>
              <div className="mt-3 rounded-lg border border-border p-3 text-xs">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-muted-foreground">{"请求数"}</span>
                  <span className="font-medium tabular-nums">{data?.request_count ?? 0}</span>
                </div>
                <div className="mt-2 flex items-center justify-between gap-2">
                  <span className="text-muted-foreground">{"状态"}</span>
                  <Badge variant={data?.last_error ? "destructive" : "outline"}>
                    {data?.last_error ? "异常" : data?.enabled === false ? "已停用" : "正常"}
                  </Badge>
                </div>
                <div className="mt-2 flex items-center justify-between gap-2">
                  <span className="text-muted-foreground">{"最后检查"}</span>
                  <span>{relativeTime(data?.last_checked_at)}</span>
                </div>
                {data?.last_error ? <p className="mt-2 line-clamp-2 text-danger">{data.last_error}</p> : null}
              </div>
              <div className="mt-auto flex items-center gap-2 pt-3">
                <Button size="sm" variant="outline" className="flex-1" onClick={() => summary.refetch()}>
                  <RefreshCw className="size-3.5" />
                  {"刷新"}
                </Button>
                <Button size="sm" className="flex-1" onClick={() => setUsersOpen(true)}>
                  {"用户明细"}
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <RelayConfigDialog open={configOpen} onOpenChange={setConfigOpen} config={config.data} />
      <RelayUsersDialog open={usersOpen} onOpenChange={setUsersOpen} />
    </>
  )
}

function Metric({ label, value, tone }: { label: string; value: string; tone: "brand" | "warning" }) {
  return (
    <div className="rounded-lg border border-border p-3">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className={cn("mt-2 text-2xl font-semibold tabular-nums", tone === "brand" ? "text-brand" : "text-warning")}>{value}</p>
    </div>
  )
}

function RelayConfigDialog({ open, onOpenChange, config }: { open: boolean; onOpenChange: (v: boolean) => void; config: RelayConfig | null }) {
  const [form, setForm] = useState<RelayForm>(() => initialForm(config))
  const [submitting, setSubmitting] = useState(false)
  const [testing, setTesting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const refresh = useTriggerRefresh()

  useEffect(() => {
    if (open) {
      setForm(initialForm(config))
      setError(null)
      setMessage(null)
    }
  }, [open, config])

  function setMultiplier(accountID: number, value: string) {
    setForm((prev) => ({
      ...prev,
      account_multipliers: prev.account_multipliers.map((m) => (m.account_id === accountID ? { ...m, multiplier: value } : m)),
    }))
  }

  async function handleTest() {
    setTesting(true)
    setError(null)
    setMessage(null)
    try {
      const result = await apiFetch<{ ok: boolean; message: string; accounts?: RelayAccount[] }>("/relay/test", {
        method: "POST",
        body: JSON.stringify(bodyFromForm(form)),
      })
      if (result.accounts?.length) {
        setForm((prev) => ({ ...prev, account_multipliers: mergeAccounts(prev.account_multipliers, result.accounts ?? []) }))
      }
      setMessage(result.message || "管理员权限校验通过")
    } catch (e) {
      setError((e as Error).message || "测试失败")
    } finally {
      setTesting(false)
    }
  }

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitting(true)
    setError(null)
    setMessage(null)
    try {
      await apiFetch<RelayConfig>("/relay/config", {
        method: "PUT",
        body: JSON.stringify(bodyFromForm(form)),
      })
      refresh()
      onOpenChange(false)
    } catch (e) {
      setError((e as Error).message || "保存失败")
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-hidden sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{"自有中转站配置"}</DialogTitle>
          <DialogDescription>{"保存前会登录 Sub2API 并检查管理员权限。"}</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex min-h-0 flex-col gap-3">
          <ScrollArea className="max-h-[60vh] pr-3">
            <div className="space-y-3">
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor="relay-name">名称</Label>
                  <Input id="relay-name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} disabled={submitting || testing} />
                </div>
                <div className="flex items-center justify-between rounded-lg border border-border px-3 py-2">
                  <div>
                    <Label>{"启用"}</Label>
                    <p className="text-[11px] text-muted-foreground">{"关闭后卡片不拉取用量"}</p>
                  </div>
                  <Switch checked={form.enabled} onCheckedChange={(v) => setForm({ ...form, enabled: v })} disabled={submitting || testing} />
                </div>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="relay-site">站点地址</Label>
                <Input id="relay-site" placeholder="https://sub2api.example.com" value={form.site_url} onChange={(e) => setForm({ ...form, site_url: e.target.value })} required disabled={submitting || testing} />
              </div>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor="relay-email">管理员邮箱</Label>
                  <Input id="relay-email" value={form.admin_email} onChange={(e) => setForm({ ...form, admin_email: e.target.value })} required disabled={submitting || testing} />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="relay-password">{config?.configured ? "管理员密码 (留空沿用)" : "管理员密码"}</Label>
                  <Input id="relay-password" type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required={!config?.configured} disabled={submitting || testing} />
                </div>
              </div>

              <div className="rounded-lg border border-border">
                <div className="flex items-center justify-between border-b border-border px-3 py-2">
                  <div>
                    <p className="text-sm font-medium">{"账号成本倍率"}</p>
                    <p className="text-[11px] text-muted-foreground">{"测试成功后自动拉取账号；成本 = 消费 / 倍率。"}</p>
                  </div>
                  <Button type="button" size="sm" variant="outline" onClick={handleTest} disabled={testing || submitting}>
                    {testing ? "测试中…" : "测试并拉取"}
                  </Button>
                </div>
                {form.account_multipliers.length === 0 ? (
                  <p className="px-3 py-6 text-sm text-muted-foreground">{"暂无账号，先点击测试并拉取。"}</p>
                ) : (
                  <div className="divide-y divide-border">
                    {form.account_multipliers.map((row) => (
                      <div key={row.account_id} className="grid grid-cols-[1fr_112px] items-center gap-3 px-3 py-2">
                        <div className="min-w-0">
                          <p className="truncate text-sm font-medium">{row.name || `账号 #${row.account_id}`}</p>
                          <p className="text-[11px] text-muted-foreground">#{row.account_id}</p>
                        </div>
                        <Input type="number" min="0.0001" step="0.0001" value={row.multiplier} onChange={(e) => setMultiplier(row.account_id, e.target.value)} disabled={submitting || testing} />
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </ScrollArea>
          {message ? <p className="text-sm text-success">{message}</p> : null}
          {error ? <p className="text-sm text-danger">{error}</p> : null}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={handleTest} disabled={testing || submitting}>
              {testing ? "测试中…" : "测试"}
            </Button>
            <Button type="submit" disabled={submitting || testing}>{submitting ? "保存中…" : "保存"}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function RelayUsersDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (v: boolean) => void }) {
  const [date, setDate] = useState(todayLocal())
  const [page, setPage] = useState(1)
  const [data, setData] = useState<RelayUsersPage | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set())

  useEffect(() => {
    if (!open) return
    let cancelled = false
    setLoading(true)
    setError(null)
    apiFetch<RelayUsersPage>(`/relay/users?date=${encodeURIComponent(date)}&page=${page}&page_size=${PAGE_SIZE}`)
      .then((res) => {
        if (cancelled) return
        setData(res)
      })
      .catch((e) => {
        if (!cancelled) setError((e as Error).message || "加载失败")
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [open, date, page])

  const totals = useMemo(() => {
    const rows = data?.items ?? []
    return rows.reduce(
      (acc, row) => ({ actual: acc.actual + row.actual_cost, cost: acc.cost + row.cost }),
      { actual: 0, cost: 0 },
    )
  }, [data])

  function toggle(row: RelayUserUsage) {
    const key = `${row.user_id}:${row.username}`
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-hidden sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>{"用户消费明细"}</DialogTitle>
          <DialogDescription>{"默认按当天用户实际消费从大到小排序。"}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Input type="date" className="w-44" value={date} onChange={(e) => { setDate(e.target.value); setPage(1) }} />
          <div className="flex gap-4 text-xs text-muted-foreground">
            <span>{"本页消费 "}<b className="text-foreground">{money(totals.actual)}</b></span>
            <span>{"本页成本 "}<b className="text-foreground">{money(totals.cost)}</b></span>
          </div>
        </div>
        <ScrollArea className="max-h-[58vh] rounded-md border border-border">
          {loading && !data ? <p className="px-4 py-6 text-sm text-muted-foreground">{"加载中…"}</p> : null}
          {!loading && (data?.items.length ?? 0) === 0 ? <p className="px-4 py-6 text-sm text-muted-foreground">{"暂无消费记录"}</p> : null}
          <div className="divide-y divide-border">
            {(data?.items ?? []).map((row) => {
              const key = `${row.user_id}:${row.username}`
              const isOpen = expanded.has(key)
              return (
                <div key={key} className="px-4 py-3">
                  <button type="button" className="flex w-full items-center gap-3 text-left" onClick={() => toggle(row)}>
                    {isOpen ? <ChevronDown className="size-4 text-muted-foreground" /> : <ChevronRight className="size-4 text-muted-foreground" />}
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-semibold">{row.username}</p>
                      <p className="text-xs text-muted-foreground">{row.main_account || "未知账号"}</p>
                    </div>
                    <div className="hidden text-right text-xs sm:block">
                      <p className="font-medium text-foreground">{row.request_count} 请求</p>
                      <p className="text-muted-foreground">{"账号 "}{row.accounts.length}</p>
                    </div>
                    <div className="w-28 text-right">
                      <p className="text-sm font-semibold tabular-nums text-brand">{money(row.actual_cost)}</p>
                      <p className="text-xs tabular-nums text-warning">{money(row.cost)}</p>
                    </div>
                  </button>
                  {isOpen ? (
                    <div className="mt-3 rounded-lg bg-muted/30">
                      {row.accounts.map((account) => (
                        <div key={`${key}:${account.account_id}`} className="grid grid-cols-[1fr_90px_90px] gap-2 px-3 py-2 text-xs sm:grid-cols-[1fr_90px_90px_90px]">
                          <span className="min-w-0 truncate">{account.account_name}</span>
                          <span className="text-right tabular-nums">{money(account.actual_cost)}</span>
                          <span className="text-right tabular-nums text-warning">{money(account.cost)}</span>
                          <span className="hidden text-right text-muted-foreground sm:block">{account.multiplier.toFixed(4)}x</span>
                        </div>
                      ))}
                    </div>
                  ) : null}
                </div>
              )
            })}
          </div>
        </ScrollArea>
        {error ? <p className="text-sm text-danger">{error}</p> : null}
        <DialogFooter className="items-center justify-between sm:justify-between">
          <span className="text-xs text-muted-foreground">{data ? `共 ${data.total} 个用户，第 ${data.page} / ${data.pages} 页` : ""}</span>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" disabled={loading || page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>{"上一页"}</Button>
            <Button variant="outline" size="sm" disabled={loading || !data || page >= data.pages} onClick={() => setPage((p) => p + 1)}>{"下一页"}</Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
