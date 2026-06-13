import React from "react"
import ReactDOM from "react-dom/client"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { Navigate, RouterProvider, createBrowserRouter } from "react-router-dom"
import { Toaster } from "@/components/ui/toaster"
import { ProtectedLayout } from "@/components/protected-layout"
import { LoginPage } from "@/pages/login"
import { MailPage } from "@/pages/mail"
import { AdminPage } from "@/pages/admin"
import { ProfilePage } from "@/pages/profile"
import "./index.css"

const queryClient = new QueryClient({ defaultOptions: { queries: { refetchOnWindowFocus: false, staleTime: 10_000 } } })
const router = createBrowserRouter([
  { path: "/login", element: <LoginPage /> },
  { path: "/", element: <ProtectedLayout />, children: [
    { index: true, element: <Navigate to="/mail" replace /> },
    { path: "mail", element: <MailPage /> },
    { path: "profile", element: <ProfilePage /> },
    { path: "admin", element: <AdminPage /> },
  ] },
])

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
      <Toaster />
    </QueryClientProvider>
  </React.StrictMode>,
)
