<template>
  <!-- Tabela de vídeos com shadcn Table: thumb, tag, status badge, tamanho, duração, data, ações -->
  <div class="rounded-md border">
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Thumb</TableHead>
          <TableHead>Tag</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Tamanho</TableHead>
          <TableHead>Duração</TableHead>
          <TableHead>Data</TableHead>
          <TableHead class="text-right">Ações</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow v-if="loading">
          <TableCell colspan="7" class="text-center py-8">
            <span class="text-muted-foreground">Carregando...</span>
          </TableCell>
        </TableRow>
        <TableRow v-else-if="!videos.length">
          <TableCell colspan="7" class="text-center py-8">
            <span class="text-muted-foreground">Nenhum vídeo encontrado.</span>
          </TableCell>
        </TableRow>
        <TableRow v-for="video in videos" :key="video.video_id">
          <!-- Thumbnail: mostra <img> se disponível, senão ícone placeholder -->
          <TableCell class="w-16">
            <div class="w-12 h-8 rounded overflow-hidden bg-muted flex items-center justify-center">
              <img
                v-if="video.thumbnail_url"
                :src="video.thumbnail_url"
                :alt="`Thumbnail de ${video.video_id}`"
                class="w-full h-full object-cover"
                loading="lazy"
                @error="(e) => { (e.target as HTMLImageElement).style.display = 'none' }"
              />
              <PhImage v-if="!video.has_thumbnails" :size="20" class="text-muted-foreground" />
            </div>
          </TableCell>
          <TableCell class="font-mono text-sm max-w-[200px] truncate">
            {{ video.tag }}
          </TableCell>
          <TableCell>
            <Badge :variant="statusVariant(video.status)">
              {{ video.status }}
            </Badge>
          </TableCell>
          <TableCell class="text-sm">
            {{ formatBytes(video.actual_size_bytes) }}
          </TableCell>
          <TableCell class="text-sm">
            {{ formatDuration(video.duration_s) }}
          </TableCell>
          <TableCell class="text-sm">
            {{ formatDate(video.created_at) }}
          </TableCell>
          <TableCell class="text-right">
            <div class="flex items-center justify-end gap-1">
              <Button
                variant="ghost"
                size="icon-sm"
                title="Ver detalhes"
                @click="$emit('view', video.video_id)"
              >
                <PhEye :size="16" />
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                title="Reprocessar"
                @click="$emit('reprocess', video.video_id)"
              >
                <PhArrowsClockwise :size="16" />
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                title="Deletar"
                @click="$emit('delete', video.video_id)"
              >
                <PhTrash :size="16" class="text-destructive" />
              </Button>
            </div>
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>
  </div>
</template>

<script setup lang="ts">
import { PhEye, PhArrowsClockwise, PhTrash, PhImage } from '@phosphor-icons/vue'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { Video } from '@/types'

defineProps<{
  videos: Video[]
  loading: boolean
}>()

defineEmits<{
  view: [videoId: string]
  reprocess: [videoId: string]
  delete: [videoId: string]
}>()

function statusVariant(status: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'ready':
      return 'default'
    case 'processing':
    case 'uploading':
      return 'secondary'
    case 'failed':
      return 'destructive'
    default:
      return 'outline'
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
  return new Date(iso).toLocaleDateString('pt-BR', {
    day: '2-digit',
    month: '2-digit',
    year: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}
</script>
