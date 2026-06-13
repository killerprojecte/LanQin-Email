import * as React from "react"
import { Navigate, Outlet, Link, useLocation, useNavigate } from "react-router-dom"
import { Inbox, LogOut, Mail, Settings } from "lucide-react"
import { useQueryClient } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { useMe } from "@/hooks/use-me"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarRail,
  SidebarTrigger,
} from "@/components/ui/sidebar"

export function ProtectedLayout() {
  const me = useMe()
  const location = useLocation()
  const navigate = useNavigate()
  const qc = useQueryClient()

  if (me.isLoading) return <div className="grid min-h-screen place-items-center text-muted-foreground">加载中...</div>
  if (me.isError || !me.data?.user) return <Navigate to="/login" replace state={{ from: location.pathname }} />

  const user = me.data.user
  const isMailRoute = location.pathname.startsWith("/mail")
  const isProfileRoute = location.pathname.startsWith("/profile")

  async function logout() {
    await api.logout().catch(() => undefined)
    qc.clear()
    navigate("/login", { replace: true })
  }

  if (isMailRoute || isProfileRoute) {
    return <Outlet />
  }

  return (
    <SidebarProvider>
      <Sidebar collapsible="icon">
        <SidebarHeader>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton size="lg" asChild>
                <Link to="/mail">
                  <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
                    <Mail className="size-4" />
                  </div>
                  <div className="grid flex-1 text-left text-sm leading-tight">
                    <span className="truncate font-semibold">LanQin Email</span>
                  </div>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupContent>
              <SidebarMenu>
                <NavItem to="/mail" icon={<Inbox />} label="Webmail" />
                {user.role === "admin" && <NavItem to="/admin" icon={<Settings />} label="系统管理" />}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
        <SidebarFooter>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton size="lg" className="group-data-[collapsible=icon]:!p-0" asChild>
                <Link to="/profile">
                  <Avatar className="h-8 w-8 rounded-lg">
                    <AvatarFallback className="rounded-lg bg-muted text-foreground">
                      {user.displayName.slice(0, 1).toUpperCase()}
                    </AvatarFallback>
                  </Avatar>
                  <div className="grid flex-1 text-left text-sm leading-tight">
                    <span className="truncate font-semibold">{user.displayName}</span>
                    <span className="truncate text-xs text-muted-foreground">{user.email}</span>
                  </div>
                  <Badge variant={user.role === "admin" ? "default" : "secondary"} className="ml-auto text-[10px]">
                    {user.role === "admin" ? "管理员" : "用户"}
                  </Badge>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
          <div className="p-2">
            <Button variant="outline" size="sm" className="w-full gap-2 text-xs" onClick={logout}>
              <LogOut className="h-3.5 w-3.5" />退出登录
            </Button>
          </div>
        </SidebarFooter>
        <SidebarRail />
      </Sidebar>
      <SidebarInset>
        <div className="flex min-h-svh flex-col bg-muted/20">
          <div className="flex h-12 items-center border-b bg-white px-3 md:hidden">
            <SidebarTrigger />
          </div>
          <Outlet />
        </div>
      </SidebarInset>
    </SidebarProvider>
  )
}

function NavItem({ to, icon, label }: { to: string; icon: React.ReactNode; label: string }) {
  const location = useLocation()
  const active = location.pathname.startsWith(to)
  return (
    <SidebarMenuItem>
      <SidebarMenuButton asChild isActive={active} tooltip={label}>
        <Link to={to}>
          {icon}
          <span>{label}</span>
        </Link>
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}
