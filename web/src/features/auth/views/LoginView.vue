<template>
  <!-- Tela de login — botão "Entrar com Google" estilizado -->
  <div class="flex min-h-screen items-center justify-center bg-background p-4">
    <div class="w-full max-w-sm space-y-6">
      <!-- Logo / título -->
      <div class="text-center space-y-2">
        <img
          :src="logoUrl"
          alt="Streamedia"
          class="mx-auto h-16 w-16 rounded-xl object-cover"
        />
        <h1 class="text-2xl font-bold tracking-tight text-foreground">
          Streamedia Admin
        </h1>
        <p class="text-sm text-muted-foreground">
          Faça login com sua conta Google para acessar o painel de administração
        </p>
      </div>

      <!-- Botão Google -->
      <Button
        class="w-full"
        size="lg"
        variant="outline"
        @click="handleLogin"
      >
        <PhGoogleLogo :size="20" weight="fill" />
        Entrar com Google
      </Button>

      <!-- Erro -->
      <p v-if="error" class="text-sm text-destructive text-center">
        {{ error }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRoute } from 'vue-router'
import { Button } from '@/components/ui/button'
import { PhGoogleLogo } from '@phosphor-icons/vue'
import logoUrl from '@/assets/logo.png'

const route = useRoute()
const error = ref<string | null>(null)

function handleLogin() {
  error.value = null

  // Guarda o redirect se existir
  const redirect = route.query.redirect as string | undefined
  if (redirect) {
    localStorage.setItem('streamedia_redirect', redirect)
  }

  // Redireciona para o endpoint de OAuth do Google
  window.location.href = '/api/auth/google'
}
</script>
