<template>

  <!-- Grid de cards de estatísticas -->

  <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">

  <StatsCard

    title="Total de Vídeos"

    :value="stats?.total_videos ?? null"

    :loading="loading"

    :icon="PhFilmReel"

  />

  <StatsCard

    title="Vídeos Prontos"

    :value="stats?.ready_videos ?? null"

    :loading="loading"

    :icon="PhCheckCircle"

  />

  <StatsCard

    title="Em Processamento"

    :value="stats?.processing_videos ?? null"

    :loading="loading"

    :icon="PhSpinnerGap"

  />

  <StatsCard

    title="Espaço Usado"

    :value="stats?.total_size_bytes ?? null"

    :loading="loading"

    format="bytes"

    :icon="PhHardDrives"

  />

  </div>

</template>



<script setup lang="ts">

import { PhFilmReel, PhCheckCircle, PhSpinnerGap, PhHardDrives } from '@phosphor-icons/vue'

import type { StatsResponse } from '@/types'

import StatsCard from './StatsCard.vue'



defineProps<{

  stats: StatsResponse | null

  loading: boolean

}>()

</script>