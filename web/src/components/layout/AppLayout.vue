<template>

  <!-- Layout principal do admin: Sidebar + Header + conteúdo -->

  <SidebarProvider>

  <div class="flex min-h-screen w-full">

    <AppSidebar />

    <div class="flex flex-1 flex-col">

    <AppHeader />

    <main class="flex-1 overflow-auto p-4 sm:p-6">

      <router-view />

    </main>

    </div>

  </div>

  </SidebarProvider>

</template>



<script setup lang="ts">

import { SidebarProvider } from '@/components/ui/sidebar'

import AppSidebar from './AppSidebar.vue'

import AppHeader from './AppHeader.vue'

</script>