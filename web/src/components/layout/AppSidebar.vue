<template>

  <!-- Sidebar lateral com menu agrupado — usa shadcn Sidebar + phosphor-icons -->

  <Sidebar collapsible="icon">

  <SidebarHeader class="flex items-center gap-2 px-4 py-3">

    <img

    :src="logoUrl"

    alt="Streamedia"

    class="h-8 w-8 rounded-md object-cover"

    />

    <span class="text-sm font-semibold group-data-[collapsible=icon]:hidden">

    Streamedia

    </span>

  </SidebarHeader>



  <SidebarContent>

    <SidebarGroup

    v-for="group in menu"

    :key="group.key"

    >

    <SidebarGroupLabel v-if="group.key !== '__root__'">

      {{ groupLabel(group.key) }}

    </SidebarGroupLabel>

    <SidebarGroupContent>

      <SidebarMenu>

      <SidebarMenuItem

        v-for="item in group.items"

        :key="item.icon + item.title"

      >

        <SidebarMenuButton

        as-child

        :is-active="isActive(item.to.name)"

        :tooltip="item.title"

        >

        <router-link :to="item.to">

          <component :is="iconComponent(item.icon)" :size="20" />

          <span>{{ item.title }}</span>

        </router-link>

        </SidebarMenuButton>

      </SidebarMenuItem>

      </SidebarMenu>

    </SidebarGroupContent>

    </SidebarGroup>

  </SidebarContent>



  <SidebarRail />

  </Sidebar>

</template>



<script setup lang="ts">

import { useRoute } from 'vue-router'

import * as PhIcons from '@phosphor-icons/vue'

import {

  Sidebar,

  SidebarContent,

  SidebarGroup,

  SidebarGroupContent,

  SidebarGroupLabel,

  SidebarHeader,

  SidebarMenu,

  SidebarMenuItem,

  SidebarMenuButton,

  SidebarRail,

} from '@/components/ui/sidebar'

import { useMenu } from '@/composables/useMenu'

import logoUrl from '@/assets/logo.png'



const { menu } = useMenu()

const route = useRoute()



/** Mapeia o nome do ícone (ex: 'ph-gauge') para o componente phosphor */

function iconComponent(iconName: string) {

  // phosphor-icons usa PascalCase: 'ph-gauge' → 'Gauge'

  const name = iconName

  .replace('ph-', '')

  .split('-')

  .map((s) => s.charAt(0).toUpperCase() + s.slice(1))

  .join('')



  return (PhIcons as Record<string, unknown>)[name] || null

}



/** Label amigável para grupos */

function groupLabel(key: string): string {

  const labels: Record<string, string> = {

  'videos-group': 'Vídeos',

  }

  return labels[key] || key

}



function isActive(routeName: string): boolean {

  return route.name === routeName

}

</script>