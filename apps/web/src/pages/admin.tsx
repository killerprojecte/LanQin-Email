import * as React from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { CheckCircle2, Copy, Globe2, Mailbox, Plus, RefreshCcw, ShieldCheck, Users } from "lucide-react"
import { api, DNSRecord, Domain } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useToast } from "@/hooks/use-toast"

export function AdminPage() {
  const domains = useQuery({ queryKey: ["admin", "domains"], queryFn: api.domains })
  const mailboxes = useQuery({ queryKey: ["admin", "mailboxes"], queryFn: api.mailboxes })
  const aliases = useQuery({ queryKey: ["admin", "aliases"], queryFn: api.aliases })
  const [selectedDomain, setSelectedDomain] = React.useState<string | null>(null)

  React.useEffect(() => {
    if (!selectedDomain && domains.data?.items?.[0]) setSelectedDomain(domains.data.items[0].id)
  }, [domains.data, selectedDomain])

  const domain = domains.data?.items.find((d) => d.id === selectedDomain)
  return (
    <ScrollArea className="h-svh">
      <div className="p-6">
        <div className="mb-6 flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">系统管理</h1>
          </div>
          <div className="flex flex-wrap gap-2">
            <CreateDomainDialog />
            <CreateMailboxDialog domains={domains.data?.items || []} />
            <CreateAliasDialog domains={domains.data?.items || []} />
          </div>
        </div>

        <div className="mb-6 grid gap-4 md:grid-cols-4">
          <Stat icon={<Globe2 />} label="域名" value={domains.data?.items.length || 0} />
          <Stat icon={<Mailbox />} label="邮箱账号" value={mailboxes.data?.items.length || 0} />
          <Stat icon={<Users />} label="别名" value={aliases.data?.items.length || 0} />
          <Stat icon={<ShieldCheck />} label="DNS 正常" value={(domains.data?.items || []).filter((d) => d.dnsStatus === "ok").length} />
        </div>

        <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_520px]">
          <Card>
            <CardHeader>
              <CardTitle>域名</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-3">
                {domains.data?.items.map((d) => (
                  <Button key={d.id} type="button" variant={selectedDomain === d.id ? "secondary" : "outline"} onClick={() => setSelectedDomain(d.id)} className="h-auto w-full justify-between p-4 text-left">
                    <div>
                      <div className="font-medium">{d.name}</div>
                      <div className="text-xs text-muted-foreground">selector: {d.dkimSelector}</div>
                    </div>
                    <Badge variant={d.dnsStatus === "ok" ? "default" : "secondary"}>{d.dnsStatus === "ok" ? "正常" : d.dnsStatus}</Badge>
                  </Button>
                ))}
              </div>
            </CardContent>
          </Card>

          <DNSPanel domain={domain} />

          <Card>
            <CardHeader>
              <CardTitle>邮箱账号</CardTitle>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>地址</TableHead>
                    <TableHead>归属用户</TableHead>
                    <TableHead>名称</TableHead>
                    <TableHead>配额</TableHead>
                    <TableHead>状态</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {mailboxes.data?.items.map((m) => (
                    <TableRow key={m.id}>
                      <TableCell className="font-medium">{m.address}</TableCell>
                      <TableCell className="text-muted-foreground">{m.userEmail || m.userId}</TableCell>
                      <TableCell>{m.displayName}</TableCell>
                      <TableCell>{m.quotaMb} MB</TableCell>
                      <TableCell><Badge variant="secondary">{m.status}</Badge></TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>别名/转发</CardTitle>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>来源</TableHead>
                    <TableHead>目标</TableHead>
                    <TableHead>状态</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {aliases.data?.items.map((a) => (
                    <TableRow key={a.id}>
                      <TableCell className="font-medium">{a.source}</TableCell>
                      <TableCell>{a.destination}</TableCell>
                      <TableCell><Badge variant={a.enabled ? "default" : "secondary"}>{a.enabled ? "启用" : "停用"}</Badge></TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </div>
      </div>
    </ScrollArea>
  )
}

function Stat({ icon, label, value }: { icon: React.ReactNode; label: string; value: number }) {
  return (
    <Card>
      <CardContent className="flex items-center gap-4 p-5">
        <div className="grid h-10 w-10 place-items-center rounded-lg bg-muted text-foreground">{icon}</div>
        <div>
          <div className="text-2xl font-semibold tracking-tight">{value}</div>
          <div className="text-xs text-muted-foreground">{label}</div>
        </div>
      </CardContent>
    </Card>
  )
}

function DNSPanel({ domain }: { domain?: Domain }) {
  const { toast } = useToast()
  const qc = useQueryClient()
  const records = useQuery({ queryKey: ["dns-records", domain?.id], queryFn: () => api.dnsRecords(domain!.id), enabled: !!domain })
  const check = useMutation({
    mutationFn: () => api.checkDns(domain!.id),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: ["admin", "domains"] })
      toast({ title: res.status === "ok" ? "DNS 检测通过" : "DNS 检测未通过", description: Object.values(res.checks).map((c) => c.message).join("；") })
    },
  })
  if (!domain) return <Card><CardContent className="p-6 text-muted-foreground">请选择域名</CardContent></Card>
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>DNS 记录</CardTitle>
          </div>
          <Button variant="outline" size="sm" onClick={() => check.mutate()} disabled={check.isPending}>
            <RefreshCcw className="h-4 w-4" />检测
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <div className="space-y-3">{records.data?.items.map((r) => <DNSRecordRow key={`${r.type}-${r.name}`} record={r} />)}</div>
        {check.data && (
          <>
            <Separator className="my-4" />
            <div className="space-y-2">
              {Object.entries(check.data.checks).map(([k, v]) => (
                <div key={k} className="flex items-center gap-2 text-sm">
                  <CheckCircle2 className={`h-4 w-4 ${v.ok ? "text-green-600" : "text-destructive"}`} />
                  <span className="font-medium">{k.toUpperCase()}:</span> {v.message}
                </div>
              ))}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}

function DNSRecordRow({ record }: { record: DNSRecord }) {
  const { toast } = useToast()
  const text = `${record.type} ${record.name} ${record.value}`
  return (
    <div className="rounded-lg border bg-card p-3">
      <div className="mb-2 flex items-center justify-between">
        <Badge variant="outline" className="font-mono">{record.type}</Badge>
        <Button size="sm" variant="ghost" className="h-7 gap-1 text-xs" onClick={() => { navigator.clipboard.writeText(text); toast({ title: "已复制" }) }}>
          <Copy className="h-3.5 w-3.5" />复制
        </Button>
      </div>
      <div className="break-all font-mono text-xs text-muted-foreground">
        <div>Name: {record.name}</div>
        <div>Value: {record.value}</div>
        <div>TTL: {record.ttl}s</div>
      </div>
    </div>
  )
}

function CreateDomainDialog() {
  const qc = useQueryClient()
  const { toast } = useToast()
  const [open, setOpen] = React.useState(false)
  const mut = useMutation({
    mutationFn: (form: FormData) => api.createDomain(String(form.get("name"))),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["admin", "domains"] }); setOpen(false); toast({ title: "域名已创建" }) },
    onError: (e) => toast({ title: "创建失败", description: e.message }),
  })
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline"><Plus className="h-4 w-4" />域名</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>添加域名</DialogTitle>
        </DialogHeader>
        <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); mut.mutate(new FormData(e.currentTarget)) }}>
          <div className="space-y-2"><Label>域名</Label><Input name="name" placeholder="example.com" required /></div>
          <DialogFooter><Button disabled={mut.isPending}>创建</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function CreateMailboxDialog({ domains }: { domains: Domain[] }) {
  const qc = useQueryClient()
  const { toast } = useToast()
  const [open, setOpen] = React.useState(false)
  const [domainId, setDomainId] = React.useState("")
  const [role, setRole] = React.useState("user")
  React.useEffect(() => { if (!domainId && domains[0]) setDomainId(domains[0].id) }, [domains, domainId])
  const mut = useMutation({
    mutationFn: (form: FormData) => api.createMailbox({
      domainId,
      localPart: String(form.get("localPart")),
      displayName: String(form.get("displayName")),
      password: String(form.get("password")),
      quotaMb: Number(form.get("quotaMb") || 1024),
      role: role as "admin" | "user",
      ownerEmail: String(form.get("ownerEmail") || ""),
    }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["admin", "mailboxes"] }); setOpen(false); toast({ title: "邮箱已创建" }) },
    onError: (e) => toast({ title: "创建失败", description: e.message }),
  })
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button><Plus className="h-4 w-4" />邮箱</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>创建邮箱账号</DialogTitle>
        </DialogHeader>
        <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); mut.mutate(new FormData(e.currentTarget)) }}>
          <DomainSelect domains={domains} value={domainId} onChange={setDomainId} />
          <div className="grid grid-cols-2 gap-3">
            <Field name="localPart" label="账号" placeholder="alice" />
            <Field name="displayName" label="显示名" placeholder="Alice" />
          </div>
          <Field name="ownerEmail" label="归属用户邮箱（可选）" placeholder="留空则使用新邮箱创建用户；填已有账号则追加邮箱" required={false} />
          <div className="grid grid-cols-2 gap-3">
            <Field name="password" label="密码" type="password" placeholder="至少 8 位" />
            <Field name="quotaMb" label="配额 MB" type="number" defaultValue="1024" />
          </div>
          <div className="space-y-2"><Label>角色</Label>
            <Select value={role} onValueChange={setRole}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="user">普通用户</SelectItem>
                <SelectItem value="admin">管理员</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <DialogFooter><Button disabled={mut.isPending || !domainId}>创建</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function CreateAliasDialog({ domains }: { domains: Domain[] }) {
  const qc = useQueryClient()
  const { toast } = useToast()
  const [open, setOpen] = React.useState(false)
  const [domainId, setDomainId] = React.useState("")
  React.useEffect(() => { if (!domainId && domains[0]) setDomainId(domains[0].id) }, [domains, domainId])
  const mut = useMutation({
    mutationFn: (form: FormData) => api.createAlias({ domainId, source: String(form.get("source")), destination: String(form.get("destination")), enabled: true }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["admin", "aliases"] }); setOpen(false); toast({ title: "别名已创建" }) },
    onError: (e) => toast({ title: "创建失败", description: e.message }),
  })
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline"><Plus className="h-4 w-4" />别名</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader><DialogTitle>创建别名/转发</DialogTitle></DialogHeader>
        <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); mut.mutate(new FormData(e.currentTarget)) }}>
          <DomainSelect domains={domains} value={domainId} onChange={setDomainId} />
          <Field name="source" label="来源" placeholder="sales 或 sales@example.com" />
          <Field name="destination" label="目标邮箱" placeholder="alice@example.com" />
          <DialogFooter><Button disabled={mut.isPending || !domainId}>创建</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function Field({ label, required = true, ...props }: React.InputHTMLAttributes<HTMLInputElement> & { label: string }) {
  return <div className="space-y-2"><Label>{label}</Label><Input required={required} {...props} /></div>
}
function DomainSelect({ domains, value, onChange }: { domains: Domain[]; value: string; onChange: (value: string) => void }) {
  return <div className="space-y-2"><Label>域名</Label>
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger><SelectValue placeholder="选择域名" /></SelectTrigger>
      <SelectContent>{domains.map((d) => <SelectItem key={d.id} value={d.id}>{d.name}</SelectItem>)}</SelectContent>
    </Select>
  </div>
}
