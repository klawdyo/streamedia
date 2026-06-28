<template>
  <!-- Detalhes de um vídeo: player, stats, timeline SSE, botões reprocessar/deletar -->
  <div class="space-y-6">
    <!-- Cabeçalho -->
    <div class="flex items-center justify-between">
      <div>
        <div class="flex items-center gap-2">
          <Button
            variant="ghost"
            size="icon-sm"
            @click="$router.back()"
          >
            <PhArrowLeft :size="18" />
          </Button>
          <h1 class="text-2xl font-bold tracking-tight">
            {{ videoStore.video?.tag || 'Carregando...' }}
          </h1>
          <Badge v-if="videoStore.video" :variant="statusVariant(videoStore.video.status)">
            {{ videoStore.video.status }}
          </Badge>
        </div>
        <p class="text-muted-foreground mt-1 ml-10 text-sm font-mono">
          {{ videoId }}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          @click="handleReprocess"
          :disabled="videoStore.loading"
        >
          <PhArrowsClockwise :size="16" class="mr-1" />
          Reprocessar
        </Button>
        <Button
          variant="destructive"
          size="sm"
          @click="handleDelete"
          :disabled="videoStore.loading"
        >
          <PhTrash :size="16" class="mr-1" />
          Deletar
        </Button>
      </div>
    </div>

    <div v-if="videoStore.loading" class="space-y-4">
      <Skeleton class="aspect-video w-full rounded-lg" />
      <Skeleton class="h-8 w-64" />
      <Skeleton class="h-4 w-full" />
    </div>

    <template v-else-if="videoStore.video">
      <!-- Player de vídeo -->
      <VideoPlayer
        v-if="playUrl"
        :src="playUrl"
        :auto-play="false"
        @error="onPlayerError"
      />
      <div v-else-if="videoStore.video.status === 'ready'" class="aspect-video bg-muted rounded-lg flex items-center justify-center">
        <Button variant="outline" @click="initPlayback">
          <PhPlay :size="16" class="mr-1" />
          Iniciar reprodução
        </Button>
      </div>
      <div v-else class="aspect-video bg-muted rounded-lg flex items-center justify-center">
        <p class="text-muted-foreground text-sm">
          Vídeo ainda não está pronto para reprodução.
        </p>
      </div>

      <!-- Stats do vídeo -->
      <Card>
        <CardHeader>
          <CardTitle>Informações do Vídeo</CardTitle>
        </CardHeader>
        <CardContent>
          <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <div>
              <span class="text-xs text-muted-foreground">Status</span>
              <p class="text-sm font-medium">{{ videoStore.video.status }}</p>
            </div>
            <div>
              <span class="text-xs text-muted-foreground">Tamanho</span>
              <p class="text-sm font-medium">{{ formatBytes(videoStore.video.actual_size_bytes) }}</p>
            </div>
            <div>
              <span class="text-xs text-muted-foreground">Duração</span>
              <p class="text-sm font-medium">{{ formatDuration(videoStore.video.duration_s) }}</p>
            </div>
            <div>
              <span class="text-xs text-muted-foreground">Criado em</span>
              <p class="text-sm font-medium">{{ formatDate(videoStore.video.created_at) }}</p>
            </div>
          </div>
        </CardContent>
      </Card>

      <!-- Timeline SSE -->
      <Card>
        <CardHeader class="flex flex-row items-center justify-between">
          <div>
            <CardTitle>Eventos em Tempo Real</CardTitle>
            <CardDescription>
              Timeline de eventos SSE do vídeo
            </CardDescription>
          </div>
          <div class="flex items-center gap-2">
            <Button
              v-if="!sse.connected.value"
              variant="outline"
              size="sm"
              @click="connectSSE"
            >
              <PhPlug :size="16" class="mr-1" />
              Conectar
            </Button>
            <Button
              v-else
              variant="outline"
              size="sm"
              @click="sse.close()"
            >
              Desconectar
            </Button>
            <Badge :variant="sse.connected.value ? 'default' : 'secondary'">
              {{ sse.connected.value ? 'Conectado' : 'Desconectado' }}
            </Badge>
          </div>
        </CardHeader>
        <CardContent>
          <div v-if="!sse.events.value.length" class="text-sm text-muted-foreground py-4 text-center">
            Nenhum evento recebido. Conecte-se ao SSE para ver eventos em tempo real.
          </div>
          <div v-else class="space-y-2 max-h-64 overflow-auto">
            <div
              v-for="(evt, idx) in sse.events.value"
              :key="idx"
              class="flex items-start gap-2 text-sm"
            >
              <span class="text-xs text-muted-foreground font-mono shrink-0">
                {{ evt.timestamp }}
              </span>
              <Badge variant="outline" class="text-xs">
                {{ evt.event }}
              </Badge>
              <span class="text-xs text-muted-foreground truncate">
                {{ evt.status || evt.tag || '—' }}
              </span>
            </div>
          </div>
        </CardContent>
      </Card>
    </template>

    <!-- Erro -->
    <div v-if="videoStore.error" class="rounded-md bg-destructive/10 p-4 text-sm text-destructive">
      {{ videoStore.error }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { PhArrowLeft, PhArrowsClockwise, PhTrash, PhPlay, PhPlug } from '@phosphor-icons/vue'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import VideoPlayer from '@/components/player/VideoPlayer.vue'
import { useVideoStore } from '../stores/video'
import { useSSE } from '@/composables/useSSE'

const route = useRoute()
const router = useRouter()
const videoStore = useVideoStore()
const videoId = route.params.id as string
const playUrl = ref<string | null>(null)
const sse = useSSE(videoId, () => localStorage.getItem('streamedia_root_token') || '')

function statusVariant(status: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'ready': return 'default'
    case 'processing': case 'uploading': return 'secondary'
    case 'failed': return 'destructive'
    default: return 'outline'
  }
}

async function initPlayback() {
    const token = localStorage.getItem('streamedia_root_token') || ''
    const data = await videoStore.initPlay(videoId, token)
    if (data) {
      playUrl.value = data.hls_url
    }
  }

function connectSSE() {
  sse.connect()
}

function onPlayerError(err: string) {
  videoStore.error = err
}

async function handleReprocess() {
  const { useVideosStore } = await import('../stores/videos')
  const videosStore = useVideosStore()
  await videosStore.reprocessVideo(videoId)
  videoStore.fetchStatus(videoId)
}

async function handleDelete() {
  if (!confirm('Tem certeza que deseja deletar este vídeo?')) return
  const { useVideosStore } = await import('../stores/videos')
  const videosStore = useVideosStore()
  const ok = await videosStore.deleteVideo(videoId)
  if (ok) {
    router.push({ name: 'videos' })
  }
}

function formatBytes(bytes?: number): string {
  if (!bytes) return '—'
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
}

function formatDuration(seconds?: number): string {
  if (!seconds) return '—'
  const m = Math.floor(seconds / 60)
  const s = Math.floor(seconds % 60)
  return `${m}:${String(s).padStart(2, '0')}`
}

function formatDate(iso: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleDateString('pt-BR')
}

onMounted(() => {
  videoStore.fetchStatus(videoId)
})

onUnmounted(() => {
  sse.close()
})
</script>
