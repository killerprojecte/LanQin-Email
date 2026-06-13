import { Navigate } from "react-router-dom"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { useMe } from "@/hooks/use-me"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useToast } from "@/hooks/use-toast"

export function LoginPage() {
  const me = useMe()
  const qc = useQueryClient()
  const { toast } = useToast()
  const login = useMutation({
    mutationFn: (form: FormData) => api.login(String(form.get("email")), String(form.get("password"))),
    onSuccess: async () => { await qc.invalidateQueries({ queryKey: ["me"] }) },
    onError: (e) => toast({ title: "登录失败", description: e.message }),
  })
  if (me.data?.user) return <Navigate to="/mail" replace />
  return (
    <div className="grid min-h-screen place-items-center bg-background px-4">
      <div className="w-full max-w-[360px]">
        <div className="mb-10 text-center">
          <h1 className="text-3xl font-bold tracking-tight">LanQin Email</h1>
        </div>
        <form className="space-y-5" onSubmit={(e) => { e.preventDefault(); login.mutate(new FormData(e.currentTarget)) }}>
          <div className="space-y-2">
            <Label htmlFor="email">邮箱</Label>
            <Input id="email" name="email" type="email" defaultValue="admin@lanqin.local" required className="h-11 text-base" />
          </div>
          <div className="space-y-2">
            <Label htmlFor="password">密码</Label>
            <Input id="password" name="password" type="password" defaultValue="ChangeMe123!" required className="h-11 text-base" />
          </div>
          <Button className="h-11 w-full text-base" disabled={login.isPending}>
            {login.isPending ? "登录中..." : "登录"}
          </Button>
        </form>
      </div>
    </div>
  )
}
