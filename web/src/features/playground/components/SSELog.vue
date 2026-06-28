<template>
  <!-- Log de eventos SSE com timestamp, tipo, ícone -->
  <div class="space-y-2 max-h-80 overflow-auto">
    <div v-if="!events.length" class="text-sm text-muted-foreground text-center py-6">
      Nenhum evento recebido.
    </div>
    <div
      v-for="(evt, idx) in events"
      :key="idx"
      class="flex items-start gap-2 rounded-md bg-muted/50 px-3 py-2 text-sm"
    >
      <!-- Ícone conforme tipo de evento -->
      <component :is="eventIcon(evt.event)" :size="16" class="mt-0.5 shrink-0" :class="eventColor(evt.event)" />

      <div class="flex-1 min-w-0">
        <div class="flex items-center gap-2">
          <Badge variant="outline" class="text-xs font-mono">
            {{ evt.event }}
          </Badge>
          <span class="text-xs text-muted-foreground font-mono">
            {{ formatTimestamp(evt.timestamp) }}
          </span>
        </div>
        <p class="text-xs mt-1 truncate">
          {{ evt.status || evt.tag || JSON.stringify(evt.data) }}
        </p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Badge } from '@/components/ui/badge'
import {
  PhInfo,
  PhWarning,
  PhCheckCircle,
  PhXCircle,
  PhSpinnerGap,
} from '@phosphor-icons/vue'
import type { SSEEvent } from '@/types'

defineProps<{
  events: SSEEvent[]
}>()

function eventIcon(event: string) {
    switch (event) {
      case 'status': return PhInfo
      case 'progress': return PhSpinnerGap
      case 'done': return PhCheckCircle
      case 'error': return PhXCircle
      default: return PhWarning
    }
  }

function eventColor(event: string): string {
  switch (event) {
    case 'status': return 'text-blue-500'
    case 'progress': return 'text-yellow-500'
    case 'done': return 'text-green-500'
    case 'error': return 'text-destructive'
    default: return 'text-muted-foreground'
  }
}

function formatTimestamp(ts: string): string {
  try {
    const d = new Date(ts)
    return d.toLocaleTimeString('pt-BR')
  } catch {
    return ts
  }
}
</script>
