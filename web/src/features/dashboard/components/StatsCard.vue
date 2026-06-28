<template>

  <!-- Card de estatística individual para o StatsGrid -->

  <Card class="overflow-hidden">

  <CardHeader class="flex flex-row items-center justify-between pb-2 space-y-0">

    <CardTitle class="text-sm font-medium text-muted-foreground">

    {{ title }}

    </CardTitle>

    <component :is="icon" :size="20" class="text-muted-foreground" />

  </CardHeader>

  <CardContent>

    <div class="text-2xl font-bold">

    <span v-if="loading" class="animate-pulse">—</span>

    <span v-else>{{ formattedValue }}</span>

    </div>

    <p v-if="subtitle" class="text-xs text-muted-foreground mt-1">

    {{ subtitle }}

    </p>

  </CardContent>

  </Card>

</template>



<script setup lang="ts">

import { computed } from 'vue'

import type { Component } from 'vue'

import {

  Card,

  CardContent,

  CardHeader,

  CardTitle,

} from '@/components/ui/card'



const props = defineProps<{

  title: string

  value: number | null

  loading: boolean

  icon: Component

  format?: 'number' | 'bytes' | 'duration'

  subtitle?: string

}>()



const formattedValue = computed(() => {

  if (props.value === null) return '—'

  switch (props.format) {

  case 'bytes':

    return formatBytes(props.value)

  case 'duration':

    return formatDuration(props.value)

  default:

    return props.value.toLocaleString()

  }

})



function formatBytes(bytes: number): string {

  if (bytes === 0) return '0 B'

  const units = ['B', 'KB', 'MB', 'GB', 'TB']

  const i = Math.floor(Math.log(bytes) / Math.log(1024))

  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`

}



function formatDuration(seconds: number): string {

  const h = Math.floor(seconds / 3600)

  const m = Math.floor((seconds % 3600) / 60)

  const s = seconds % 60

  if (h > 0) return `${h}h ${m}m`

  if (m > 0) return `${m}m ${s}s`

  return `${s}s`

}

</script>