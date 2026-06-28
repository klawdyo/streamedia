// Fetch wrapper centralizado — token compatível com streamedia_root_token,

// CSRF em métodos não-GET, redireciona para login em 401.



import type { ApiResponse } from '@/types'



const API_BASE = ''



function getToken(): string | null {

  return localStorage.getItem('streamedia_root_token') || localStorage.getItem('auth_token')

}



function clearToken() {

  localStorage.removeItem('streamedia_root_token')

  localStorage.removeItem('auth_token')

}



async function request<T>(

  url: string,

  options: RequestInit = {},

): Promise<ApiResponse<T>> {

  const token = getToken()

  const method = options.method || 'GET'



  const headers: HeadersInit = {

  'Content-Type': 'application/json',

  ...(token && { Authorization: `Bearer ${token}` }),

  ...(method !== 'GET' && { 'X-Streamedia-Csrf': '1' }),

  ...options.headers,

  }



  const response = await fetch(`${API_BASE}${url}`, {

  ...options,

  method,

  headers,

  })



  // 401 — redireciona para login

  if (response.status === 401) {

  clearToken()

  // Só redireciona se não estiver já na tela de auth

  if (!window.location.pathname.includes('/app/auth')) {

    window.location.href = `/app/auth?redirect=${encodeURIComponent(window.location.pathname + window.location.search)}`

  }

  return { data: null as unknown as T, error: 'Não autenticado' }

  }



  if (!response.ok) {

  const body = await response.json().catch(() => ({}))

  // body.error é boolean (true) no envelope da API; body.message é a string descritiva

  const msg = (typeof body.error === 'string' ? body.error : body.message) || `Erro HTTP ${response.status}`

  return {

    data: null as unknown as T,

    error: msg,

  }

  }



  const json = await response.json()

  return json

}



export const api = {

  get: <T>(url: string) => request<T>(url),



  post: <T>(url: string, body?: unknown) =>

  request<T>(url, { method: 'POST', body: body !== undefined ? JSON.stringify(body) : undefined }),



  put: <T>(url: string, body?: unknown) =>

  request<T>(url, { method: 'PUT', body: body !== undefined ? JSON.stringify(body) : undefined }),



  del: <T>(url: string) =>

  request<T>(url, { method: 'DELETE' }),

}