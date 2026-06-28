<template>
  <!-- Header do admin: avatar, nome, dropdown (logout), ThemeToggle, toggle sidebar mobile -->
  <header class="sticky top-0 z-30 flex h-14 items-center gap-4 border-b bg-background px-4 sm:px-6">
    <!-- Botão toggle sidebar (mobile) -->
    <SidebarTrigger class="-ml-2 sm:hidden" />

    <div class="flex-1" />

    <!-- Toggle de tema -->
    <ThemeToggle />

    <!-- Avatar + nome + dropdown logout -->
    <DropdownMenu>
      <DropdownMenuTrigger as-child>
        <button class="flex items-center gap-2 rounded-md px-2 py-1 hover:bg-accent transition-colors">
          <Avatar class="h-8 w-8">
            <AvatarImage v-if="auth.user?.picture" :src="auth.user.picture" />
            <AvatarFallback class="text-sm">
              {{ initials }}
            </AvatarFallback>
          </Avatar>
          <span class="hidden text-sm font-medium md:inline-block">
            {{ auth.user?.name || auth.user?.email }}
          </span>
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" class="w-48">
        <DropdownMenuLabel>{{ auth.user?.email }}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem @click="handleLogout">
          <PhSignOut :size="16" class="mr-2" />
          Sair
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  </header>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { PhSignOut } from '@phosphor-icons/vue'
import { SidebarTrigger } from '@/components/ui/sidebar'
import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useAuthStore } from '@/stores/auth'
import ThemeToggle from './ThemeToggle.vue'

const auth = useAuthStore()
const router = useRouter()

const initials = computed(() => {
  const name = auth.user?.name
  if (!name) return auth.user?.email?.charAt(0).toUpperCase() || '?'
  return name
    .split(' ')
    .map((n) => n.charAt(0))
    .join('')
    .toUpperCase()
    .slice(0, 2)
})

function handleLogout() {
  auth.logout()
  router.push('/auth')
}
</script>
