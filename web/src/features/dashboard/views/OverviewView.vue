<template>

  <!-- Dashboard / Visão Geral — stats, fila, eventos recentes -->

  <div class="space-y-6">

  <div>

    <h1 class="text-2xl font-bold tracking-tight">Visão Geral</h1>

    <p class="text-muted-foreground mt-1">

    Monitoramento do servidor de mídia Streamedia

    </p>

  </div>



  <!-- Stats Grid -->

  <StatsGrid :stats="statsStore.stats" :loading="statsStore.loadingStats" />



  <!-- Fila + Status dos vídeos -->

  <div class="grid gap-4 md:grid-cols-2">

    <!-- Queue widget -->

    <Card>

    <CardHeader>

      <CardTitle class="flex items-center gap-2">

      <PhQueue :size="20" />

      Fila de Processamento

      </CardTitle>

      <CardDescription>

      Vídeos aguardando ou em processamento

      </CardDescription>

    </CardHeader>

    <CardContent class="space-y-4">

      <div v-if="statsStore.loadingQueue" class="space-y-2">

      <Skeleton class="h-4 w-full" />

      <Skeleton class="h-4 w-3/4" />

      </div>

      <div v-else-if="statsStore.queue" class="space-y-3">

      <div class="flex items-center justify-between">

        <span class="text-sm text-muted-foreground">Na fila</span>

        <Badge variant="secondary">{{ statsStore.queue.queue_length }}</Badge>

      </div>

      <div class="flex items-center justify-between">

        <span class="text-sm text-muted-foreground">Processando</span>

        <Badge variant="default">{{ statsStore.queue.processing }}</Badge>

      </div>

      <div class="flex items-center justify-between">

        <span class="text-sm text-muted-foreground">Pendentes</span>

        <Badge variant="outline">{{ statsStore.queue.pending }}</Badge>

      </div>

      </div>

      <p v-else class="text-sm text-muted-foreground">

      Nenhum dado disponível.

      </p>

    </CardContent>

    </Card>



    <!-- Status dos vídeos -->

    <Card>

    <CardHeader>

      <CardTitle class="flex items-center gap-2">

      <PhChartBar :size="20" />

      Status dos Vídeos

      </CardTitle>

      <CardDescription>

      Distribuição por status

      </CardDescription>

    </CardHeader>

    <CardContent>

      <div v-if="statsStore.loadingStats" class="space-y-3">

      <Skeleton class="h-8 w-full" v-for="i in 4" :key="i" />

      </div>

      <div v-else-if="statsStore.stats" class="space-y-3">

      <div class="flex items-center justify-between">

        <div class="flex items-center gap-2">

        <div class="h-3 w-3 rounded-full bg-green-500" />

        <span class="text-sm">Prontos</span>

        </div>

        <span class="text-sm font-medium">{{ statsStore.stats.ready_videos }}</span>

      </div>

      <div class="flex items-center justify-between">

        <div class="flex items-center gap-2">

        <div class="h-3 w-3 rounded-full bg-yellow-500" />

        <span class="text-sm">Processando</span>

        </div>

        <span class="text-sm font-medium">{{ statsStore.stats.processing_videos }}</span>

      </div>

      <div class="flex items-center justify-between">

        <div class="flex items-center gap-2">

        <div class="h-3 w-3 rounded-full bg-red-500" />

        <span class="text-sm">Falharam</span>

        </div>

        <span class="text-sm font-medium">{{ statsStore.stats.failed_videos }}</span>

      </div>

      <Separator />

      <div class="flex items-center justify-between">

        <span class="text-sm font-medium">Total</span>

        <span class="text-sm font-medium">{{ statsStore.stats.total_videos }}</span>

      </div>

      </div>

    </CardContent>

    </Card>

  </div>



  <!-- Últimos eventos SSE (placeholder) -->

  <Card>

    <CardHeader>

    <CardTitle class="flex items-center gap-2">

      <PhBroadcast :size="20" />

      Eventos Recentes

    </CardTitle>

    <CardDescription>

      Últimos 5 eventos do servidor

    </CardDescription>

    </CardHeader>

    <CardContent>

    <p class="text-sm text-muted-foreground">

      Conecte-se a um vídeo via SSE para ver eventos em tempo real.

    </p>

    </CardContent>

  </Card>

  </div>

</template>



<script setup lang="ts">

import { onMounted } from 'vue'

import { PhQueue, PhChartBar, PhBroadcast } from '@phosphor-icons/vue'

import {

  Card,

  CardContent,

  CardDescription,

  CardHeader,

  CardTitle,

} from '@/components/ui/card'

import { Badge } from '@/components/ui/badge'

import { Separator } from '@/components/ui/separator'

import { Skeleton } from '@/components/ui/skeleton'

import { useStatsStore } from '../stores/stats'

import StatsGrid from '../components/StatsGrid.vue'



const statsStore = useStatsStore()



onMounted(() => {

  statsStore.fetchAll()

})

</script>