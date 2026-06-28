// Router com rotas do admin unificado — extende RouteMeta com permissões e menu.
// Rotas protegidas usam AppLayout como wrapper.
// /app/auth é pública (sem layout).

import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'

// Extende a interface RouteMeta do vue-router
declare module 'vue-router' {
  interface RouteMeta {
    title: string
    permissions: string[]
    showInMenu: boolean
    icon: string
    parent?: string
    order?: number
  }
}

const protectedRoutes: RouteRecordRaw[] = [
  {
    path: '',
    redirect: '/app/overview',
  },
  {
    path: 'overview',
    name: 'overview',
    component: () => import('@/features/dashboard/views/OverviewView.vue'),
    meta: {
      title: 'Visão Geral',
      permissions: [],
      showInMenu: true,
      icon: 'ph-gauge',
      order: 1,
    },
  },
  {
    path: 'videos',
    name: 'videos',
    component: () => import('@/features/videos/views/VideosView.vue'),
    meta: {
      title: 'Vídeos',
      permissions: [],
      showInMenu: true,
      icon: 'ph-film-reel',
      parent: 'videos-group',
      order: 1,
    },
  },
  {
    path: 'videos/:id',
    name: 'video-detail',
    component: () => import('@/features/videos/views/VideoView.vue'),
    meta: {
      title: 'Detalhes do Vídeo',
      permissions: [],
      showInMenu: false,
      icon: 'ph-film-reel',
      order: 0,
    },
  },
  {
    path: 'playground',
    name: 'playground',
    component: () => import('@/features/playground/views/PlaygroundView.vue'),
    meta: {
      title: 'Playground',
      permissions: [],
      showInMenu: true,
      icon: 'ph-flask',
      parent: 'videos-group',
      order: 2,
    },
  },
  {
    path: 'users',
    name: 'users',
    component: () => import('@/features/users/views/UsersView.vue'),
    meta: {
      title: 'Usuários',
      permissions: ['dev', 'admin', 'acl'],
      showInMenu: true,
      icon: 'ph-users',
      order: 3,
    },
  },
  {
    path: 'config',
    name: 'config',
    component: () => import('@/features/config/views/ConfigView.vue'),
    meta: {
      title: 'Configurações',
      permissions: ['dev', 'admin'],
      showInMenu: true,
      icon: 'ph-gear',
      order: 4,
    },
  },
]

const routes: RouteRecordRaw[] = [
  {
    path: '/app',
    component: () => import('@/components/layout/AppLayout.vue'),
    children: protectedRoutes,
  },
  {
    path: '/app/auth',
    name: 'login',
    component: () => import('@/features/auth/views/LoginView.vue'),
    meta: {
      title: 'Login',
      permissions: [],
      showInMenu: false,
      icon: '',
      order: 0,
    },
  },
  {
    path: '/',
    redirect: '/app',
  },
]

const router = createRouter({
  history: createWebHistory('/app/'),
  routes,
})

export default router
