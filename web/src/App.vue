<template>

  <div class="min-h-screen bg-background text-foreground antialiased">

  <router-view />

  <ToastProvider />

  </div>

</template>



<script setup lang="ts">

import { useTheme } from '@/composables/useTheme'

import ToastProvider from '@/components/ui/toast/ToastProvider.vue'



// Inicializa o tema ao montar a aplicação

useTheme()

</script>