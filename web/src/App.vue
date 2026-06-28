<template>
  <div class="min-h-screen bg-background text-foreground antialiased">
    <router-view />
  </div>
</template>

<script setup lang="ts">
import { useTheme } from '@/composables/useTheme'

// Inicializa o tema ao montar a aplicação
useTheme()
</script>
