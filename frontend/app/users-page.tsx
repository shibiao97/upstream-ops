import { useState } from "react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useConfirm } from "@/components/ui/confirm-dialog"
import { apiFetch } from "@/lib/api"
import { useAuth } from "@/lib/auth-context"
import { useUsers } from "@/lib/queries"

export default function UsersPage() {
  const { isSuperAdmin } = useAuth()
  const [search, setSearch] = useState("")
  const users = useUsers(search, isSuperAdmin)
  const { confirm, dialog } = useConfirm()

  if (!isSuperAdmin) {
    return <section className="text-sm text-muted-foreground">只有超级管理员可以管理用户。</section>
  }

  async function act(path: string, ok: string) {
    try {
      await apiFetch(path, { method: "POST" })
      toast.success(ok)
      users.refetch()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "操作失败")
    }
  }

  return (
    <section className="space-y-3">
      <header className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="text-lg font-semibold text-foreground">用户管理</h1>
          <p className="text-xs text-muted-foreground">注册用户默认普通用户；删除会级联清理其渠道和通知渠道。</p>
        </div>
        <Input className="h-8 max-w-xs text-xs" placeholder="搜索账号" value={search} onChange={(e) => setSearch(e.target.value)} />
      </header>

      <Card className="overflow-hidden border-border shadow-none">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>账号</TableHead>
              <TableHead>角色</TableHead>
              <TableHead>状态</TableHead>
              <TableHead className="text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {(users.data ?? []).map((u) => (
              <TableRow key={u.id}>
                <TableCell className="font-medium">{u.username}</TableCell>
                <TableCell>{u.role === "super_admin" ? "超级管理员" : "普通用户"}</TableCell>
                <TableCell>{u.enabled ? "启用" : "停用"}</TableCell>
                <TableCell className="space-x-2 text-right">
                  {u.role === "super_admin" ? null : (
                    <>
                      <Button size="sm" variant="outline" onClick={() => void act(`/users/${u.id}/${u.enabled ? "disable" : "enable"}`, u.enabled ? "已停用" : "已启用")}>
                        {u.enabled ? "停用" : "启用"}
                      </Button>
                      <Button
                        size="sm"
                        variant="destructive"
                        onClick={async () => {
                          const ok = await confirm({
                            title: `删除用户 ${u.username}？`,
                            description: "会级联删除该用户的渠道、通知渠道和相关历史数据，无法恢复。",
                            confirmLabel: "删除",
                            destructive: true,
                          })
                          if (!ok) return
                          try {
                            await apiFetch(`/users/${u.id}`, { method: "DELETE" })
                            toast.success("已删除")
                            users.refetch()
                          } catch (e) {
                            toast.error(e instanceof Error ? e.message : "删除失败")
                          }
                        }}
                      >
                        删除
                      </Button>
                    </>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
      {dialog}
    </section>
  )
}
