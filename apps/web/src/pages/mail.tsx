import * as React from "react"
import DOMPurify from "dompurify"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useNavigate, useSearchParams } from "react-router-dom"
import type { ImperativePanelHandle } from "react-resizable-panels"
import { Archive, Check, ChevronsUpDown, Copy, Forward, Inbox, Mail, MailCheck, Moon, PanelLeftClose, PanelLeftOpen, Paperclip, PencilLine, RefreshCcw, Reply, Search, Send, Settings, SlidersHorizontal, Star, Sun, Trash2 } from "lucide-react"
import { api, Mailbox, MailMessage } from "@/lib/api"
import { cn, formatBytes, formatDate } from "@/lib/utils"
import { applyTheme, getInitialTheme } from "@/lib/theme"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Sheet, SheetContent, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable"
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
} from "@/components/ui/sidebar"
import { useMe } from "@/hooks/use-me"
import { useToast } from "@/hooks/use-toast"

const folderIcons: Record<string, React.ReactNode> = { inbox: <Inbox className="h-4 w-4" />, sent: <Send className="h-4 w-4" />, archive: <Archive className="h-4 w-4" />, trash: <Trash2 className="h-4 w-4" /> }
const folderLabels: Record<string, string> = {
  Inbox: "收件箱",
  Sent: "已发送",
  Drafts: "草稿箱",
  Archive: "归档",
  Spam: "垃圾邮件",
  Trash: "回收站",
}

type ComposeDraft = { key: string; to?: string; cc?: string; bcc?: string; subject?: string; text?: string }
type MailFilter = "all" | "unread" | "starred" | "attachments"

const filterLabels: Record<MailFilter, string> = {
  all: "全部邮件",
  unread: "未读邮件",
  starred: "星标邮件",
  attachments: "有附件",
}

export function MailPage() {
  const qc = useQueryClient()
  const { toast } = useToast()
  const navigate = useNavigate()
  const me = useMe()
  const [searchParams, setSearchParams] = useSearchParams()
  const [folder, setFolder] = React.useState(() => searchParams.get("folder") || "Inbox")
  const [query, setQuery] = React.useState("")
  const [selectedId, setSelectedId] = React.useState<string | null>(null)
  const [composeOpen, setComposeOpen] = React.useState(false)
  const [composeDraft, setComposeDraft] = React.useState<ComposeDraft | undefined>()
  const [sidebarCollapsed, setSidebarCollapsed] = React.useState(false)
  const [mailFilter, setMailFilter] = React.useState<MailFilter>("all")
  const [selectedMailboxId, setSelectedMailboxId] = React.useState(() => localStorage.getItem("lanqin:selected-mailbox") || "")
  const [darkMode, setDarkMode] = React.useState(getInitialTheme)
  const sidebarPanelRef = React.useRef<ImperativePanelHandle>(null)
  const themeMountedRef = React.useRef(false)

  const mailboxList = useQuery({ queryKey: ["mailboxes", "mine"], queryFn: api.myMailboxes })
  const selectedMailbox = React.useMemo(() => mailboxList.data?.items.find((item) => item.id === selectedMailboxId), [mailboxList.data?.items, selectedMailboxId])
  const folders = useQuery({ queryKey: ["folders", selectedMailboxId], queryFn: () => api.folders(selectedMailboxId), enabled: !!selectedMailboxId })
  const messages = useQuery({ queryKey: ["messages", selectedMailboxId, folder, query], queryFn: () => api.messages(folder, query, "", selectedMailboxId), enabled: !!selectedMailboxId })
  const detail = useQuery({ queryKey: ["message", selectedId], queryFn: () => api.message(selectedId!), enabled: !!selectedId })
  const star = useMutation({ mutationFn: ({ id, starred }: { id: string; starred: boolean }) => api.star(id, starred), onSuccess: () => qc.invalidateQueries({ queryKey: ["messages"] }) })
  const del = useMutation({ mutationFn: (id: string) => api.delete(id), onSuccess: async () => { setSelectedId(null); await qc.invalidateQueries({ queryKey: ["messages"] }); await qc.invalidateQueries({ queryKey: ["folders"] }); toast({ title: "已删除" }) } })
  const move = useMutation({ mutationFn: ({ id, folder }: { id: string; folder: string }) => api.move(id, folder), onSuccess: async () => { setSelectedId(null); await qc.invalidateQueries({ queryKey: ["messages"] }); await qc.invalidateQueries({ queryKey: ["folders"] }); toast({ title: "已移动" }) } })
  const markAllRead = useMutation({
    mutationFn: async (items: MailMessage[]) => {
      const unread = items.filter((message) => !message.isRead)
      await Promise.all(unread.map((message) => api.markRead(message.id, true)))
      return unread.length
    },
    onSuccess: async (count) => {
      await qc.invalidateQueries({ queryKey: ["messages"] })
      await qc.invalidateQueries({ queryKey: ["folders"] })
      toast({ title: count > 0 ? `已标记 ${count} 封邮件为已读` : "当前没有未读邮件" })
    },
    onError: (error) => toast({ title: "操作失败", description: error.message }),
  })

  React.useEffect(() => {
    const items = mailboxList.data?.items || []
    if (items.length === 0) return
    if (!selectedMailboxId || !items.some((item) => item.id === selectedMailboxId)) {
      setSelectedMailboxId(items[0].id)
    }
  }, [mailboxList.data?.items, selectedMailboxId])

  React.useEffect(() => {
    if (selectedMailboxId) localStorage.setItem("lanqin:selected-mailbox", selectedMailboxId)
  }, [selectedMailboxId])

  React.useEffect(() => {
    const nextFolder = searchParams.get("folder") || "Inbox"
    if (nextFolder !== folder) {
      setFolder(nextFolder)
      setSelectedId(null)
    }
  }, [folder, searchParams])

  React.useEffect(() => {
    applyTheme(darkMode, themeMountedRef.current)
    themeMountedRef.current = true
  }, [darkMode])

  React.useEffect(() => {
    const events = new EventSource("/api/events", { withCredentials: true })
    events.addEventListener("sync", () => qc.invalidateQueries({ queryKey: ["folders"] }))
    return () => events.close()
  }, [qc])

  const selected = detail.data
  const allMessages = messages.data?.items || []
  const visibleMessages = allMessages.filter((message) => {
    if (mailFilter === "unread") return !message.isRead
    if (mailFilter === "starred") return message.isStarred
    if (mailFilter === "attachments") return message.hasAttachments
    return true
  })
  const unreadCount = allMessages.filter((message) => !message.isRead).length
  function openCompose(draft?: ComposeDraft) { setComposeDraft(draft || { key: `new-${Date.now()}` }); setComposeOpen(true) }
  function openReply(message: MailMessage) { openCompose({ key: `reply-${message.id}-${Date.now()}`, to: message.from, subject: withPrefix(message.subject, "Re:"), text: quoteMessage(message) }) }
  function openForward(message: MailMessage) { openCompose({ key: `forward-${message.id}-${Date.now()}`, subject: withPrefix(message.subject, "Fwd:"), text: quoteMessage(message) }) }
  function switchMailbox(mailboxId: string) {
    setSelectedMailboxId(mailboxId)
    setFolder("Inbox")
    setSearchParams({})
    setSelectedId(null)
    setMailFilter("all")
  }
  async function copyCurrentMailbox() {
    if (!selectedMailbox?.address) return
    await navigator.clipboard.writeText(selectedMailbox.address)
    toast({ title: "邮箱地址已复制" })
  }
  function openSettings() {
    navigate("/profile")
  }
  function toggleSidebar() {
    if (sidebarCollapsed) {
      sidebarPanelRef.current?.expand(14)
      setSidebarCollapsed(false)
    } else {
      sidebarPanelRef.current?.collapse()
      setSidebarCollapsed(true)
    }
  }

  return (
    <div className="h-svh bg-background">
      <SidebarProvider className="h-full min-h-0 w-full">
        <ResizablePanelGroup direction="horizontal" className="h-full min-h-0 w-full">
            <ResizablePanel ref={sidebarPanelRef} collapsible collapsedSize={4} defaultSize={15} minSize={11} maxSize={24} onCollapse={() => setSidebarCollapsed(true)} onExpand={() => setSidebarCollapsed(false)}>
                <Sidebar collapsible="none" className="h-full w-full border-r bg-sidebar">
                  <SidebarHeader className={cn("border-b py-3", sidebarCollapsed ? "px-2" : "px-3")}>
                  <AccountHeader
                    collapsed={sidebarCollapsed}
                    name={me.data?.user.displayName || selectedMailbox?.address || "LanQin"}
                    email={me.data?.user.email || selectedMailbox?.address}
                    darkMode={darkMode}
                    onToggleTheme={() => setDarkMode((value) => !value)}
                    onSettings={openSettings}
                  />
                  <div className={cn("mt-2 flex gap-2", sidebarCollapsed && "justify-center")}>
                    <MailboxSwitcher
                      collapsed={sidebarCollapsed}
                      mailboxes={mailboxList.data?.items || []}
                      selectedMailbox={selectedMailbox}
                      onSelect={switchMailbox}
                    />
                    {!sidebarCollapsed && (
                      <Button type="button" variant="outline" size="icon" className="h-9 w-9 shrink-0 rounded-md" onClick={copyCurrentMailbox} disabled={!selectedMailbox}>
                        <Copy className="h-4 w-4" />
                      </Button>
                    )}
                  </div>
                  <Button className={cn("mt-2 h-10 w-full rounded-md text-sm", sidebarCollapsed && "px-0")} size={sidebarCollapsed ? "icon" : "default"} onClick={() => openCompose()}>
                    <PencilLine className="h-4 w-4" />
                    {!sidebarCollapsed && <span>写邮件</span>}
                  </Button>
                </SidebarHeader>
                <SidebarContent>
                  <SidebarGroup>
                    {!sidebarCollapsed && <SidebarGroupLabel>邮件夹</SidebarGroupLabel>}
                    <SidebarGroupContent>
                      <SidebarMenu>
                        {(folders.data?.items || []).map((f) => (
                          <SidebarMenuItem key={f.id}>
                            <SidebarMenuButton isActive={folder === f.name} className={cn(sidebarCollapsed && "justify-center px-0")} onClick={() => { setFolder(f.name); setSearchParams(f.name === "Inbox" ? {} : { folder: f.name }); setSelectedId(null) }}>
                              {folderIcons[f.role] || <Inbox className="h-4 w-4" />}
                              {!sidebarCollapsed && <span>{folderLabels[f.name] || f.name}</span>}
                              {!sidebarCollapsed && f.unreadCount > 0 && <Badge variant="secondary" className="ml-auto">{f.unreadCount}</Badge>}
                            </SidebarMenuButton>
                          </SidebarMenuItem>
                        ))}
                      </SidebarMenu>
                      {folders.isLoading && <FolderSkeleton />}
                    </SidebarGroupContent>
                  </SidebarGroup>
                </SidebarContent>
                <div className={cn("mt-auto border-t p-2", sidebarCollapsed ? "flex justify-center" : "")}>
                  <Button type="button" variant="ghost" size={sidebarCollapsed ? "icon" : "sm"} className={cn(!sidebarCollapsed && "w-full justify-start")} onClick={toggleSidebar}>
                    {sidebarCollapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
                    {!sidebarCollapsed && <span>收起侧栏</span>}
                  </Button>
                </div>
              </Sidebar>
            </ResizablePanel>
            <ResizableHandle withHandle />

            <ResizablePanel defaultSize={85} minSize={60}>
              <section className="flex h-full min-h-0 flex-col">
                <header className="flex h-16 shrink-0 items-center justify-between gap-3 border-b px-5">
                  <div className="flex items-center gap-2">
                    <Button size="icon" variant="ghost" onClick={() => { qc.invalidateQueries({ queryKey: ["messages"] }); qc.invalidateQueries({ queryKey: ["folders"] }) }}><RefreshCcw className="h-4 w-4" /></Button>
                    <Button variant="outline" size="sm" disabled={markAllRead.isPending || unreadCount === 0} onClick={() => markAllRead.mutate(allMessages)}><MailCheck className="h-4 w-4" />全部已读</Button>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="outline" size="sm"><SlidersHorizontal className="h-4 w-4" />{filterLabels[mailFilter]}</Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="start">
                        {(Object.keys(filterLabels) as MailFilter[]).map((value) => (
                          <DropdownMenuItem key={value} onSelect={() => setMailFilter(value)}>
                            {filterLabels[value]}
                          </DropdownMenuItem>
                        ))}
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                  <div className="relative w-full max-w-md">
                    <Search className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" />
                    <Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="搜索邮件" className="pl-9" />
                  </div>
                </header>

                <ResizablePanelGroup direction="horizontal" className="min-h-0 flex-1">
                  <ResizablePanel defaultSize={32} minSize={24} maxSize={44}>
                    <div className="flex h-full min-h-0 flex-col">
                      <div className="flex h-14 shrink-0 items-center justify-between border-b px-5">
                        <div>
                          <div className="text-sm font-semibold">{folderLabels[folder] || folder}</div>
                          <div className="text-xs text-muted-foreground">{visibleMessages.length} / {allMessages.length} 封邮件</div>
                        </div>
                      </div>
                      <ScrollArea className="min-h-0 flex-1">
                        {messages.isLoading && <MessageSkeleton />}
                        {visibleMessages.map((m) => <MessageRow key={m.id} message={m} active={selectedId === m.id} onClick={() => setSelectedId(m.id)} onStar={() => star.mutate({ id: m.id, starred: !m.isStarred })} />)}
                        {!messages.isLoading && visibleMessages.length === 0 && <div className="p-8 text-center text-sm text-muted-foreground">{allMessages.length === 0 ? "当前文件夹没有邮件" : "当前筛选条件下没有邮件"}</div>}
                      </ScrollArea>
                    </div>
                  </ResizablePanel>
                  <ResizableHandle withHandle />

                  <ResizablePanel defaultSize={68} minSize={44}>
                    <section className="h-full min-h-0">
                      {!selectedId && <div className="grid h-full place-items-center text-muted-foreground">选择一封邮件阅读</div>}
                      {detail.isLoading && <div className="space-y-4 p-6"><Skeleton className="h-8 w-2/3" /><Skeleton className="h-4 w-1/3" /><Separator /><Skeleton className="h-40 w-full" /></div>}
                      {selected && <div className="flex h-full min-h-0 flex-col">
                        <div className="border-b p-5">
                          <div className="mb-4 flex items-center justify-between gap-3">
                            <h2 className="text-xl font-semibold">{selected.subject}</h2>
                            <div className="flex flex-wrap justify-end gap-2">
                              <Button variant="outline" size="sm" onClick={() => openReply(selected)}><Reply className="h-4 w-4" />回复</Button>
                              <Button variant="outline" size="sm" onClick={() => openForward(selected)}><Forward className="h-4 w-4" />转发</Button>
                              {selected.folder === "Archive" ? (
                                <Button variant="outline" size="sm" onClick={() => move.mutate({ id: selected.id, folder: "Inbox" })}>取消归档</Button>
                              ) : (
                                <Button variant="outline" size="sm" onClick={() => move.mutate({ id: selected.id, folder: "Archive" })}>归档</Button>
                              )}
                              <Button variant="destructive" size="sm" onClick={() => del.mutate(selected.id)}>删除</Button>
                            </div>
                          </div>
                          <div className="text-sm text-muted-foreground"><span className="font-medium text-foreground">{selected.from}</span> 发给 {selected.to.join(", ")} · {formatDate(selected.receivedAt)}</div>
                        </div>
                        <ScrollArea className="min-h-0 flex-1">
                          <div className="p-6">
                            <div className="mail-html prose max-w-none text-sm leading-7" dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(selected.bodyHtml || `<pre>${selected.bodyText || ""}</pre>`) }} />
                            {selected.attachments && selected.attachments.length > 0 && <div className="mt-8 rounded-lg border p-4"><div className="mb-3 font-medium">附件</div><div className="space-y-2">{selected.attachments.map((a) => <a className="flex items-center justify-between rounded-md border p-3 text-sm hover:bg-accent" href={`/api/mail/attachments/${a.id}`} key={a.id}><span className="flex items-center gap-2"><Paperclip className="h-4 w-4" />{a.filename}</span><span className="text-muted-foreground">{formatBytes(a.sizeBytes)}</span></a>)}</div></div>}
                          </div>
                        </ScrollArea>
                      </div>}
                    </section>
                  </ResizablePanel>
                </ResizablePanelGroup>
              </section>
            </ResizablePanel>
          </ResizablePanelGroup>
      </SidebarProvider>

      <ComposeSheet mailbox={selectedMailbox} open={composeOpen} draft={composeDraft} onOpenChange={(open) => { setComposeOpen(open); if (!open) setComposeDraft(undefined) }} onSent={() => { setComposeOpen(false); setComposeDraft(undefined); qc.invalidateQueries({ queryKey: ["messages"] }); qc.invalidateQueries({ queryKey: ["folders"] }) }} />
    </div>
  )
}

function FolderSkeleton() { return <div className="space-y-2 p-2"><Skeleton className="h-8 w-full" /><Skeleton className="h-8 w-4/5" /><Skeleton className="h-8 w-3/4" /></div> }
function MessageSkeleton() { return <div className="space-y-0">{Array.from({ length: 6 }).map((_, i) => <div className="space-y-2 border-b p-4" key={i}><Skeleton className="h-4 w-1/2" /><Skeleton className="h-4 w-4/5" /><Skeleton className="h-3 w-full" /></div>)}</div> }

function AccountHeader({ collapsed, name, email, darkMode, onToggleTheme, onSettings }: { collapsed: boolean; name: string; email?: string; darkMode: boolean; onToggleTheme: () => void; onSettings: () => void }) {
  const displayName = cleanAccountName(name, email)
  if (collapsed) {
    return (
      <div className="flex justify-center">
        <Avatar className="size-8 rounded-full">
          <AvatarFallback className="bg-primary text-xs font-semibold text-primary-foreground">{accountInitial(displayName, email)}</AvatarFallback>
        </Avatar>
      </div>
    )
  }
  return (
    <div className="flex items-center justify-between gap-2">
      <div className="flex min-w-0 items-center gap-2">
        <Avatar className="size-8 rounded-full">
          <AvatarFallback className="bg-primary text-xs font-semibold text-primary-foreground">{accountInitial(displayName, email)}</AvatarFallback>
        </Avatar>
        <div className="min-w-0 text-sm">
          <div className="truncate text-sm font-semibold leading-5">{displayName}</div>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        <Button type="button" variant="ghost" size="icon" className="size-8 rounded-md text-muted-foreground" onClick={onToggleTheme}>
          {darkMode ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
        </Button>
        <Button type="button" variant="ghost" size="icon" className="size-8 rounded-md text-muted-foreground" onClick={onSettings}>
          <Settings className="h-4 w-4" />
        </Button>
      </div>
    </div>
  )
}

function MailboxSwitcher({ collapsed, mailboxes, selectedMailbox, onSelect }: { collapsed: boolean; mailboxes: Mailbox[]; selectedMailbox?: Mailbox; onSelect: (mailboxId: string) => void }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" className={cn("h-9 min-w-0 flex-1 justify-start gap-2 rounded-md bg-background px-2 text-left font-normal", collapsed && "w-8 flex-none justify-center px-0")}>
          <Mail className="h-4 w-4 shrink-0 text-muted-foreground" />
          {!collapsed && (
            <>
              <span className="min-w-0 flex-1 truncate text-sm font-medium">{selectedMailbox?.address || "选择邮箱"}</span>
              <ChevronsUpDown className="h-4 w-4 shrink-0 text-muted-foreground" />
            </>
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-72">
        {mailboxes.length === 0 && <DropdownMenuItem disabled>没有可用邮箱</DropdownMenuItem>}
        {mailboxes.map((mailbox) => (
          <DropdownMenuItem key={mailbox.id} onSelect={() => onSelect(mailbox.id)} className="gap-2">
            <Check className={cn("h-4 w-4", selectedMailbox?.id === mailbox.id ? "opacity-100" : "opacity-0")} />
            <span className="min-w-0 flex-1 truncate font-medium">{mailbox.address}</span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function cleanAccountName(name: string, email?: string) {
  const value = name.trim()
  if (!value || (email && value.toLowerCase() === email.toLowerCase())) return email?.split("@")[0] || "用户"
  return value
}

function accountInitial(name: string, email?: string) {
  const source = cleanAccountName(name, email)
  const first = Array.from(source.trim())[0]
  return (first || "蓝").toUpperCase()
}

function MessageRow({ message, active, onClick, onStar }: { message: MailMessage; active: boolean; onClick: () => void; onStar: () => void }) {
  return <div onClick={onClick} className={cn("cursor-pointer border-b p-4 transition-colors hover:bg-accent/50", active && "bg-accent", !message.isRead && "font-semibold")}>
    <div className="mb-1 flex items-center justify-between gap-2"><div className="truncate text-sm">{message.from}</div><div className="shrink-0 text-xs text-muted-foreground">{formatDate(message.receivedAt)}</div></div>
    <div className="mb-1 flex items-center gap-2"><Button type="button" variant="ghost" size="icon" className="h-6 w-6 text-muted-foreground hover:text-yellow-500" onClick={(e) => { e.stopPropagation(); onStar() }}><Star className={cn("h-4 w-4", message.isStarred && "fill-yellow-400 text-yellow-500")} /></Button><span className="truncate text-sm">{message.subject}</span>{message.hasAttachments && <Paperclip className="h-3 w-3 text-muted-foreground" />}</div>
    <div className="line-clamp-2 text-xs text-muted-foreground">{message.snippet}</div>
  </div>
}

function ComposeSheet({ mailbox, open, draft, onOpenChange, onSent }: { mailbox?: Mailbox; open: boolean; draft?: ComposeDraft; onOpenChange: (v: boolean) => void; onSent: () => void }) {
  const { toast } = useToast()
  const [files, setFiles] = React.useState<File[]>([])
  const send = useMutation({ mutationFn: api.send, onSuccess: () => { toast({ title: "发送成功" }); setFiles([]); onSent() }, onError: (e) => toast({ title: "发送失败", description: e.message }) })
  async function submit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (!mailbox) {
      toast({ title: "请选择发件邮箱" })
      return
    }
    const form = new FormData(e.currentTarget)
    const attachments = await Promise.all(files.map(fileToAttachment))
    const text = String(form.get("text") || "")
    send.mutate({ mailboxId: mailbox.id, to: splitEmails(String(form.get("to") || "")), cc: splitEmails(String(form.get("cc") || "")), bcc: splitEmails(String(form.get("bcc") || "")), subject: String(form.get("subject") || ""), text, html: text.replace(/\n/g, "<br>"), attachments })
  }
  return <Sheet open={open} onOpenChange={onOpenChange}><SheetContent className="overflow-y-auto sm:max-w-2xl"><SheetHeader><SheetTitle>写信</SheetTitle></SheetHeader><form key={draft?.key || "new"} className="mt-5 space-y-4" onSubmit={submit}>
    <div className="space-y-2"><Label>发件邮箱</Label><Input value={mailbox?.address || "未选择"} readOnly /></div>
    <div className="space-y-2"><Label>收件人</Label><Input name="to" placeholder="user@example.com, other@example.com" defaultValue={draft?.to || ""} required /></div>
    <div className="grid grid-cols-2 gap-3"><div className="space-y-2"><Label>抄送</Label><Input name="cc" defaultValue={draft?.cc || ""} /></div><div className="space-y-2"><Label>密送</Label><Input name="bcc" defaultValue={draft?.bcc || ""} /></div></div>
    <div className="space-y-2"><Label>主题</Label><Input name="subject" defaultValue={draft?.subject || ""} /></div>
    <div className="space-y-2"><Label>正文</Label><Textarea name="text" className="min-h-[220px]" defaultValue={draft?.text || ""} /></div>
    <div className="space-y-2"><Label>附件</Label><Input type="file" multiple onChange={(e) => setFiles(Array.from(e.currentTarget.files || []))} />{files.length > 0 && <div className="text-xs text-muted-foreground">{files.map((f) => `${f.name} (${formatBytes(f.size)})`).join("，")}</div>}</div>
    <SheetFooter><Button type="button" variant="outline" onClick={() => onOpenChange(false)}>取消</Button><Button disabled={send.isPending || !mailbox}>{send.isPending ? "发送中..." : "发送"}</Button></SheetFooter>
  </form></SheetContent></Sheet>
}

function splitEmails(s: string) { return s.split(/[;,\s]+/).map((v) => v.trim()).filter(Boolean) }
function withPrefix(subject: string, prefix: string) { return subject.toLowerCase().startsWith(prefix.toLowerCase()) ? subject : `${prefix} ${subject}` }
function quoteMessage(message: MailMessage) {
  const body = message.bodyText || stripHtml(message.bodyHtml || message.snippet || "")
  const quote = body.split("\n").map((line) => `> ${line}`).join("\n")
  return `\n\n----- 原始邮件 -----\nFrom: ${message.from}\nTo: ${message.to.join(", ")}\nDate: ${formatDate(message.receivedAt)}\nSubject: ${message.subject}\n\n${quote}`
}
function stripHtml(html: string) { const div = document.createElement("div"); div.innerHTML = DOMPurify.sanitize(html); return div.textContent || div.innerText || "" }
async function fileToAttachment(file: File) {
  const buffer = await file.arrayBuffer()
  let binary = ""
  const bytes = new Uint8Array(buffer)
  for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i])
  return { filename: file.name, contentType: file.type || "application/octet-stream", contentBase64: btoa(binary) }
}
