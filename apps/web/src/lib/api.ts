export type User = { id: string; email: string; displayName: string; role: "admin" | "user"; disabled: boolean; createdAt: string }
export type Domain = { id: string; name: string; status: string; dkimSelector: string; dkimPublicKey?: string; dnsStatus: string; dnsCheckedAt?: string; createdAt: string }
export type Mailbox = { id: string; userId: string; userEmail?: string; domainId: string; localPart: string; address: string; displayName: string; quotaMb: number; status: string; createdAt: string }
export type Alias = { id: string; domainId: string; source: string; destination: string; enabled: boolean; createdAt: string }
export type MailFolder = { id: string; name: string; role: string; unreadCount: number; totalCount: number }
export type Attachment = { id: string; messageId: string; filename: string; contentType: string; sizeBytes: number; createdAt: string }
export type MailMessage = {
  id: string; mailboxId?: string; folderId: string; folder: string; messageUid: string; messageId: string; subject: string; from: string; to: string[]; cc: string[]; bcc?: string[]; sentAt: string; receivedAt: string; snippet: string; bodyText?: string; bodyHtml?: string; isRead: boolean; isStarred: boolean; hasAttachments: boolean; sizeBytes: number; attachments?: Attachment[]
}
export type DNSRecord = { type: string; name: string; value: string; ttl: number }
export type DNSCheckResult = { domain: string; status: string; checks: Record<string, { ok: boolean; message: string; found?: string[] }> }
export type ListResponse<T> = { items: T[]; nextCursor?: string }
export type SendPayload = { mailboxId?: string; to: string[]; cc: string[]; bcc: string[]; subject: string; text: string; html: string; attachments: { filename: string; contentType: string; contentBase64: string }[] }
export type Contact = { id: string; name: string; email: string; note: string; createdAt: string }
export type MailRule = { id: string; mailboxId: string; name: string; fromContains: string; subjectContains: string; action: "archive" | "trash" | "star" | "mark-read"; enabled: boolean; createdAt: string }
export type BlockedSender = { id: string; mailboxId: string; email: string; reason: string; createdAt: string }
export type MailStats = { totalMessages: number; unreadMessages: number; starredMessages: number; attachmentCount: number; storageBytes: number; byFolder: { folder: string; role: string; count: number; unread: number; bytes: number }[] }

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(path, { credentials: "include", headers: { "Content-Type": "application/json", ...(init.headers || {}) }, ...init })
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`
    try { const body = await res.json(); message = body.error || message } catch {}
    throw new Error(message)
  }
  return res.json() as Promise<T>
}

export const api = {
  login: (email: string, password: string) => request<{ user: User }>("/api/auth/login", { method: "POST", body: JSON.stringify({ email, password }) }),
  logout: () => request<{ ok: boolean }>("/api/auth/logout", { method: "POST" }),
  me: () => request<{ user: User }>("/api/me"),
  updateProfile: (payload: { displayName: string }) => request<{ user: User }>("/api/me/profile", { method: "POST", body: JSON.stringify(payload) }),
  changePassword: (payload: { currentPassword: string; newPassword: string }) => request<{ ok: boolean }>("/api/me/password", { method: "POST", body: JSON.stringify(payload) }),
  contacts: () => request<ListResponse<Contact>>("/api/me/contacts"),
  createContact: (payload: { name: string; email: string; note: string }) => request<Contact>("/api/me/contacts", { method: "POST", body: JSON.stringify(payload) }),
  deleteContact: (id: string) => request<{ ok: boolean }>(`/api/me/contacts/${id}`, { method: "DELETE" }),
  rules: () => request<ListResponse<MailRule>>("/api/me/rules"),
  createRule: (payload: { mailboxId: string; name: string; fromContains: string; subjectContains: string; action: string; enabled: boolean }) => request<MailRule>("/api/me/rules", { method: "POST", body: JSON.stringify(payload) }),
  deleteRule: (id: string) => request<{ ok: boolean }>(`/api/me/rules/${id}`, { method: "DELETE" }),
  blockedSenders: () => request<ListResponse<BlockedSender>>("/api/me/blocked-senders"),
  createBlockedSender: (payload: { mailboxId: string; email: string; reason: string }) => request<BlockedSender>("/api/me/blocked-senders", { method: "POST", body: JSON.stringify(payload) }),
  deleteBlockedSender: (id: string) => request<{ ok: boolean }>(`/api/me/blocked-senders/${id}`, { method: "DELETE" }),
  mailStats: (mailboxId?: string) => request<MailStats>(`/api/me/stats${mailboxId ? `?mailboxId=${encodeURIComponent(mailboxId)}` : ""}`),
  cleanupMail: (payload: { mailboxId: string; target: "empty-trash" | "empty-spam" | "archive-read-inbox" }) => request<{ ok: boolean; affected: number }>("/api/me/cleanup", { method: "POST", body: JSON.stringify(payload) }),
  domains: () => request<ListResponse<Domain>>("/api/admin/domains"),
  createDomain: (name: string) => request<Domain>("/api/admin/domains", { method: "POST", body: JSON.stringify({ name }) }),
  mailboxes: () => request<ListResponse<Mailbox>>("/api/admin/mailboxes"),
  createMailbox: (payload: { domainId: string; localPart: string; displayName: string; password: string; quotaMb: number; role: "admin" | "user"; ownerEmail?: string }) => request<Mailbox>("/api/admin/mailboxes", { method: "POST", body: JSON.stringify(payload) }),
  aliases: () => request<ListResponse<Alias>>("/api/admin/aliases"),
  createAlias: (payload: { domainId: string; source: string; destination: string; enabled: boolean }) => request<Alias>("/api/admin/aliases", { method: "POST", body: JSON.stringify(payload) }),
  dnsRecords: (domainId: string) => request<{ items: DNSRecord[] }>(`/api/admin/domains/${domainId}/dns-records`),
  checkDns: (domainId: string) => request<DNSCheckResult>(`/api/admin/domains/${domainId}/check-dns`, { method: "POST" }),
  myMailboxes: () => request<ListResponse<Mailbox>>("/api/mail/mailboxes"),
  folders: (mailboxId?: string) => request<ListResponse<MailFolder>>(`/api/mail/folders${mailboxId ? `?mailboxId=${encodeURIComponent(mailboxId)}` : ""}`),
  messages: (folder: string, q = "", cursor = "", mailboxId?: string) => {
    const params = new URLSearchParams({ folder, q, cursor })
    if (mailboxId) params.set("mailboxId", mailboxId)
    return request<ListResponse<MailMessage>>(`/api/mail/messages?${params.toString()}`)
  },
  message: (id: string) => request<MailMessage>(`/api/mail/messages/${id}`),
  send: (payload: SendPayload) => request<MailMessage>("/api/mail/send", { method: "POST", body: JSON.stringify(payload) }),
  markRead: (id: string, read: boolean) => request<{ ok: boolean }>(`/api/mail/messages/${id}/mark-read`, { method: "POST", body: JSON.stringify({ read }) }),
  star: (id: string, starred: boolean) => request<{ ok: boolean }>(`/api/mail/messages/${id}/star`, { method: "POST", body: JSON.stringify({ starred }) }),
  move: (id: string, folder: string) => request<{ ok: boolean }>(`/api/mail/messages/${id}/move`, { method: "POST", body: JSON.stringify({ folder }) }),
  delete: (id: string) => request<{ ok: boolean }>(`/api/mail/messages/${id}`, { method: "DELETE" }),
}
