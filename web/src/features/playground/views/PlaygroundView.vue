<template>
  <!-- Playground completo: Upload Init → Chunks → SSE → Resultados -->
  <div class="space-y-6">
    <div>
      <h1 class="text-2xl font-bold tracking-tight">API Playground</h1>
      <p class="text-muted-foreground mt-1">
        Fluxo completo de upload: init, chunks, transcodificação e playback
      </p>
    </div>

    <!-- ====== STEP 1: Upload Init ====== -->
    <Card>
      <CardHeader>
        <CardTitle class="flex items-center gap-2">
          <span class="flex items-center justify-center w-6 h-6 rounded-full bg-primary text-primary-foreground text-xs font-bold">1</span>
          Upload Init
        </CardTitle>
        <CardDescription>Inicia o upload e recebe a URL TUS e o token</CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <div class="flex items-end gap-3 flex-wrap">
          <div class="flex-1 min-w-[200px]">
            <label class="text-xs font-medium text-muted-foreground mb-1 block">Tag (namespace)</label>
            <Input v-model="tag" placeholder="meu-video" />
          </div>
          <div class="flex-1 min-w-[200px]">
            <label class="text-xs font-medium text-muted-foreground mb-1 block">Arquivo</label>
            <Input type="file" accept="video/*" @change="onFileSelected" />
          </div>
          <Button @click="step1Init" :disabled="!canInit || step1loading">
            <PhPlay :size="16" class="mr-1" />
            {{ step1loading ? 'Enviando...' : 'Iniciar Upload' }}
          </Button>
        </div>

        <!-- Request -->
        <div v-if="step1Request" class="space-y-2">
          <h4 class="text-xs font-semibold text-muted-foreground">Request</h4>
          <div class="rounded-md bg-muted p-3 text-xs font-mono space-y-1 overflow-auto max-h-48">
            <div><span class="text-blue-400">POST</span> {{ step1Request.url }}</div>
            <div v-for="(v, k) in step1Request.headers" :key="k" class="text-muted-foreground">{{ k }}: {{ v }}</div>
            <div class="mt-2 text-foreground">{{ step1Request.body }}</div>
          </div>
        </div>

        <!-- Response -->
        <div v-if="step1Response" class="space-y-2">
          <div class="flex items-center gap-2">
            <h4 class="text-xs font-semibold text-muted-foreground">Response</h4>
            <Badge :variant="step1Ok ? 'default' : 'destructive'" class="text-xs">{{ step1Response.status }}</Badge>
            <span v-if="step1Duration != null" class="text-xs text-muted-foreground">{{ step1Duration }}ms</span>
          </div>
          <div class="rounded-md bg-muted p-3 text-xs font-mono space-y-1 overflow-auto max-h-48">
            <div v-for="(v, k) in step1Response.headers" :key="k" class="text-muted-foreground">{{ k }}: {{ v }}</div>
            <div class="mt-2 text-foreground whitespace-pre-wrap">{{ step1Response.body }}</div>
          </div>
        </div>
      </CardContent>
    </Card>

    <!-- ====== STEP 2: Upload em Chunks ====== -->
    <Card v-if="step1Ok">
      <CardHeader>
        <CardTitle class="flex items-center gap-2">
          <span class="flex items-center justify-center w-6 h-6 rounded-full bg-primary text-primary-foreground text-xs font-bold">2</span>
          Upload em Chunks (TUS)
        </CardTitle>
        <CardDescription>
          URL: <code class="text-xs bg-muted px-1 rounded">{{ uploadUrl }}</code>
        </CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <div class="flex items-end gap-3 flex-wrap">
          <div class="w-32">
            <label class="text-xs font-medium text-muted-foreground mb-1 block">Chunks</label>
            <Input v-model.number="chunkCount" type="number" min="1" max="20" />
          </div>
          <span class="text-xs text-muted-foreground pb-2">
            ~{{ formatBytes(chunkSize) }} por chunk ({{ formatBytes(selectedFile?.size || 0) }} total)
          </span>
          <Button @click="step2Upload" :disabled="step2uploading">
            <PhUpload :size="16" class="mr-1" />
            {{ step2uploading ? 'Enviando...' : 'Enviar Chunks' }}
          </Button>
        </div>

        <!-- Overall progress -->
        <div v-if="step2started" class="space-y-1">
          <div class="flex justify-between text-xs text-muted-foreground">
            <span>Progresso geral</span>
            <span>{{ overallProgress }}%</span>
          </div>
          <div class="w-full bg-muted rounded-full h-2 overflow-hidden">
            <div class="bg-primary h-full rounded-full transition-all duration-300" :style="{ width: overallProgress + '%' }"></div>
          </div>
        </div>

        <!-- Per-chunk progress -->
        <div v-if="chunks.length" class="space-y-2 max-h-80 overflow-auto">
          <div v-for="(chunk, i) in chunks" :key="i" class="border rounded-md p-3 space-y-1">
            <div class="flex items-center justify-between">
              <span class="text-xs font-medium">Chunk {{ i + 1 }}/{{ chunks.length }}</span>
              <Badge :variant="chunk.status === 'done' ? 'default' : chunk.status === 'error' ? 'destructive' : 'secondary'" class="text-xs">
                {{ chunk.status === 'uploading' ? `${chunk.progress}%` : chunk.status }}
              </Badge>
            </div>
            <div class="w-full bg-muted rounded-full h-1.5 overflow-hidden">
              <div
                class="h-full rounded-full transition-all duration-200"
                :class="chunk.status === 'done' ? 'bg-green-500' : chunk.status === 'error' ? 'bg-destructive' : 'bg-primary'"
                :style="{ width: chunk.progress + '%' }"
              ></div>
            </div>
            <div class="text-xs text-muted-foreground">
              Offset: {{ chunk.offset }} | Tamanho: {{ formatBytes(chunk.size) }} | HTTP {{ chunk.responseStatus || '—' }}
            </div>
            <div v-if="chunk.responseBody" class="rounded bg-muted p-2 text-xs font-mono text-muted-foreground whitespace-pre-wrap max-h-24 overflow-auto">
              {{ chunk.responseBody }}
            </div>
          </div>
        </div>
      </CardContent>
    </Card>

    <!-- ====== STEP 3: SSE / Transcodificação ====== -->
    <Card v-if="uploadDone">
      <CardHeader>
        <CardTitle class="flex items-center gap-2">
          <span class="flex items-center justify-center w-6 h-6 rounded-full bg-primary text-primary-foreground text-xs font-bold">3</span>
          Transcodificação (SSE)
        </CardTitle>
        <CardDescription>
          Acompanhando processamento de <code class="text-xs bg-muted px-1 rounded">{{ videoId }}</code>
        </CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <div class="flex items-center gap-2">
          <Button v-if="!sseConnected" variant="outline" size="sm" @click="connectSSE">
            <PhPlug :size="16" class="mr-1" /> Conectar SSE
          </Button>
          <Button v-else variant="outline" size="sm" @click="disconnectSSE">
            Desconectar
          </Button>
          <Badge :variant="sseConnected ? 'default' : 'secondary'" class="text-xs">
            {{ sseConnected ? 'Conectado' : 'Desconectado' }}
          </Badge>
          <Badge v-if="transcodeStatus" variant="outline" class="text-xs">{{ transcodeStatus }}</Badge>
        </div>

        <div v-if="!sseEvents.length && !sseConnected" class="text-sm text-muted-foreground py-4 text-center">
          Conecte ao SSE para acompanhar a transcodificação em tempo real
        </div>

        <div v-else class="space-y-1 max-h-64 overflow-auto">
          <div v-for="(evt, i) in sseEvents" :key="i" class="flex items-start gap-2 text-sm py-1 border-b border-border/50">
            <span class="text-xs text-muted-foreground font-mono shrink-0 w-20">{{ formatTime(evt.timestamp) }}</span>
            <Badge variant="outline" class="text-xs shrink-0">{{ evt.event }}</Badge>
            <span class="text-xs text-muted-foreground truncate">{{ evt.status || evt.tag || evt.data ? JSON.stringify(evt.data) : '' }}</span>
          </div>
        </div>
      </CardContent>
    </Card>

    <!-- ====== STEP 4: Resultados ====== -->
    <Card v-if="transcodeStatus === 'ready'">
      <CardHeader>
        <CardTitle class="flex items-center gap-2">
          <span class="flex items-center justify-center w-6 h-6 rounded-full bg-green-500 text-white text-xs font-bold">4</span>
          Resultados
        </CardTitle>
        <CardDescription>Vídeo processado — thumbnails e player com métricas</CardDescription>
      </CardHeader>
      <CardContent class="space-y-6">
        <!-- Thumbnails -->
        <div v-if="videoThumbnails && Object.keys(videoThumbnails).length" class="space-y-2">
          <h4 class="text-xs font-semibold text-muted-foreground">Thumbnails por resolução</h4>
          <div class="flex gap-3 flex-wrap">
            <div
              v-for="(url, res) in videoThumbnails"
              :key="res"
              class="rounded-lg overflow-hidden border bg-muted cursor-pointer hover:ring-2 hover:ring-primary transition-all"
              :class="{ 'ring-2 ring-primary': activeResolution === res }"
              @click="selectResolution(res, url)"
            >
              <img :src="url" :alt="`${res}p`" class="h-20 w-auto object-cover" loading="lazy" />
              <div class="text-center text-xs py-1 font-medium">{{ res }}p</div>
            </div>
          </div>
        </div>

        <!-- Player -->
        <div v-if="playUrl" class="space-y-2">
          <div class="flex items-center gap-3 flex-wrap">
            <Button @click="startPlayback">
              <PhPlay :size="16" class="mr-1" /> Iniciar Reprodução
            </Button>
            <span v-if="loadTimeMs != null" class="text-xs text-muted-foreground">
              Tempo até iniciar: <span class="font-mono font-medium">{{ loadTimeMs }}ms</span>
            </span>
            <span v-if="activeResolution" class="text-xs text-muted-foreground">
              Resolução: <span class="font-mono font-medium">{{ activeResolution }}p</span>
            </span>
          </div>

          <div v-if="playerVisible" class="relative aspect-video bg-black rounded-lg overflow-hidden">
            <video
              ref="playerRef"
              :src="playUrl"
              class="w-full h-full"
              controls
              autoplay
              @loadeddata="onPlayerLoaded"
              @error="onPlayerError"
            ></video>
          </div>

          <!-- Métricas do vídeo -->
          <div v-if="videoStats" class="grid gap-3 sm:grid-cols-3">
            <div class="rounded-md bg-muted p-3">
              <span class="text-xs text-muted-foreground">Duração</span>
              <p class="text-sm font-medium">{{ formatDuration(videoStats.duration_s) }}</p>
            </div>
            <div class="rounded-md bg-muted p-3">
              <span class="text-xs text-muted-foreground">Tamanho</span>
              <p class="text-sm font-medium">{{ formatBytes(videoStats.actual_size_bytes) }}</p>
            </div>
            <div class="rounded-md bg-muted p-3">
              <span class="text-xs text-muted-foreground">Resoluções</span>
              <p class="text-sm font-medium">{{ videoStats.resolutions?.join('p, ') || '—' }}p</p>
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onUnmounted } from 'vue'
import { PhPlay, PhUpload, PhPlug } from '@phosphor-icons/vue'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from '@/components/ui/card'
import type { SSEEvent, VideoStatusResponse, PlayInitResponse } from '@/types'

// =========== STEP 1: Upload Init ===========
const tag = ref('')
const selectedFile = ref<File | null>(null)
const step1loading = ref(false)
const step1Request = ref<{ url: string; headers: Record<string, string>; body: string } | null>(null)
const step1Response = ref<{ status: string; headers: Record<string, string>; body: string } | null>(null)
const step1Duration = ref<number | null>(null)
const step1Ok = ref(false)
const uploadUrl = ref('')
const uploadToken = ref('')
const videoId = ref('')

const canInit = computed(() => tag.value.trim() && selectedFile.value)

function onFileSelected(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files?.length) selectedFile.value = input.files[0]
}

async function step1Init() {
  if (!canInit.value) return
  step1loading.value = true
  step1Request.value = null
  step1Response.value = null
  step1Ok.value = false

  const url = '/api/upload/init'
  const body = JSON.stringify({ tag: tag.value.trim(), declared_size_bytes: selectedFile.value!.size })
  const token = localStorage.getItem('streamedia_root_token') || ''
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${token}`,
    'X-Streamedia-Csrf': '1',
  }

  step1Request.value = { url, headers: { ...headers }, body }

  const start = performance.now()
  try {
    const resp = await fetch(url, { method: 'POST', headers, body })
    const respHeaders: Record<string, string> = {}
    resp.headers.forEach((v, k) => { respHeaders[k] = v })
    const respBody = await resp.text()
    step1Duration.value = Math.round(performance.now() - start)
    step1Response.value = { status: `${resp.status} ${resp.statusText}`, headers: respHeaders, body: tryPretty(respBody) }

    if (resp.ok) {
      const data = JSON.parse(respBody)
      // Envelope: { data: { video_id, tag, upload_url, token } }
      const inner = data.data || data
      uploadUrl.value = inner.upload_url
      uploadToken.value = inner.token
      videoId.value = inner.video_id
      step1Ok.value = true
    }
  } catch (e) {
    step1Duration.value = Math.round(performance.now() - start)
    step1Response.value = { status: 'Erro de rede', headers: {}, body: String(e) }
  }
  step1loading.value = false
}

// =========== STEP 2: Upload em Chunks ===========
const chunkCount = ref(4)
const step2uploading = ref(false)
const step2started = ref(false)
const uploadDone = ref(false)

interface ChunkState {
  index: number; offset: number; size: number; progress: number
  status: 'pending' | 'uploading' | 'done' | 'error'
  responseStatus: string; responseBody: string
}
const chunks = ref<ChunkState[]>([])

const chunkSize = computed(() => {
  const file = selectedFile.value
  if (!file) return 0
  return Math.ceil(file.size / Math.max(1, chunkCount.value))
})

const overallProgress = computed(() => {
  if (!chunks.value.length) return 0
  const total = chunks.value.reduce((s, c) => s + c.progress, 0)
  return Math.round(total / chunks.value.length)
})

async function step2Upload() {
  if (!selectedFile.value || !uploadUrl.value) return
  step2uploading.value = true
  step2started.value = true
  uploadDone.value = false

  const file = selectedFile.value
  const count = Math.max(1, chunkCount.value)
  const cSize = Math.ceil(file.size / count)

  chunks.value = []
  for (let i = 0; i < count; i++) {
    const offset = i * cSize
    const end = Math.min(offset + cSize, file.size)
    chunks.value.push({ index: i, offset, size: end - offset, progress: 0, status: 'pending', responseStatus: '', responseBody: '' })
  }

  // Envia chunks sequencialmente
  for (const chunk of chunks.value) {
    chunk.status = 'uploading'
    const blob = file.slice(chunk.offset, chunk.offset + chunk.size)
    try {
      await sendChunk(chunk, blob)
      chunk.status = 'done'
      chunk.progress = 100
    } catch (e) {
      chunk.status = 'error'
      chunk.responseBody = String(e)
    }
  }

  step2uploading.value = false
  const allOk = chunks.value.every(c => c.status === 'done')
  if (allOk) uploadDone.value = true
}

function sendChunk(chunk: ChunkState, blob: Blob): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('PATCH', uploadUrl.value)

    xhr.upload.addEventListener('progress', (e) => {
      if (e.lengthComputable) chunk.progress = Math.round((e.loaded / e.total) * 100)
    })

    xhr.addEventListener('load', () => {
      chunk.responseStatus = `${xhr.status} ${xhr.statusText}`
      chunk.responseBody = tryPretty(xhr.responseText)
      if (xhr.status >= 200 && xhr.status < 300) resolve()
      else reject(new Error(`HTTP ${xhr.status}`))
    })

    xhr.addEventListener('error', () => {
      chunk.responseStatus = 'Erro de rede'
      reject(new Error('Erro de rede'))
    })

    xhr.setRequestHeader('Content-Type', 'application/offset+octet-stream')
    xhr.setRequestHeader('Upload-Offset', String(chunk.offset))
    xhr.setRequestHeader('Tus-Resumable', '1.0.0')
    if (uploadToken.value) xhr.setRequestHeader('Authorization', `Bearer ${uploadToken.value}`)
    xhr.send(blob)
  })
}

// =========== STEP 3: SSE ===========
const sseEvents = ref<SSEEvent[]>([])
const sseConnected = ref(false)
const transcodeStatus = ref('')
let eventSource: EventSource | null = null

function connectSSE() {
  if (eventSource) return
  const token = localStorage.getItem('streamedia_root_token') || ''
  const url = `/api/events?video_id=${videoId.value}&token=${encodeURIComponent(token)}`
  eventSource = new EventSource(url)
  sseConnected.value = true

  eventSource.onmessage = (e) => {
    try {
      const evt: SSEEvent = JSON.parse(e.data)
      sseEvents.value.push(evt)
      if (evt.status) transcodeStatus.value = evt.status
      if (evt.status === 'ready' || evt.status?.startsWith('failed')) fetchVideoStatus()
    } catch { /* ignora parse errors */ }
  }

  eventSource.onerror = () => {
    sseConnected.value = false
    eventSource?.close()
    eventSource = null
  }
}

function disconnectSSE() {
  eventSource?.close()
  eventSource = null
  sseConnected.value = false
}

onUnmounted(() => disconnectSSE())

// =========== STEP 4: Resultados ===========
const playerRef = ref<HTMLVideoElement | null>(null)
const playerVisible = ref(false)
const playUrl = ref('')
const loadTimeMs = ref<number | null>(null)
const activeResolution = ref('')
const videoThumbnails = ref<Record<string, string> | null>(null)
const videoStats = ref<VideoStatusResponse | null>(null)

async function fetchVideoStatus() {
  const resp = await fetch(`/api/status/${videoId.value}`, {
    headers: { Authorization: `Bearer ${localStorage.getItem('streamedia_root_token') || ''}` }
  })
  if (resp.ok) {
    const json = await resp.json()
    const data = json.data || json
    videoStats.value = data
    videoThumbnails.value = data.thumbnails || null
    transcodeStatus.value = data.status
  }
}

function selectResolution(res: string, _url: string) {
  activeResolution.value = res
  playerVisible.value = false
  playUrl.value = ''
  loadTimeMs.value = null
}

async function startPlayback() {
  playerVisible.value = true
  loadTimeMs.value = null
  const clickTime = performance.now()
  const token = localStorage.getItem('streamedia_root_token') || ''

  try {
    const resp = await fetch('/api/play/init', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
        'X-Streamedia-Csrf': '1',
      },
      body: JSON.stringify({ video_id: videoId.value, token }),
    })
    if (resp.ok) {
      const json = await resp.json()
      const data: PlayInitResponse = json.data || json
      playUrl.value = data.hls_url
      // O tempo será medido no evento loadeddata do <video>
      ;(window as any).__playbackClickTime = clickTime
    }
  } catch { /* erro já visível no player */ }
}

function onPlayerLoaded() {
  const clickTime = (window as any).__playbackClickTime
  if (clickTime) {
    loadTimeMs.value = Math.round(performance.now() - clickTime)
    delete (window as any).__playbackClickTime
  }
}

function onPlayerError() {
  loadTimeMs.value = null
}

// =========== Utilitários ===========
function tryPretty(text: string): string {
  try { return JSON.stringify(JSON.parse(text), null, 2) }
  catch { return text }
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

function formatTime(iso: string): string {
  try { return new Date(iso).toLocaleTimeString('pt-BR') }
  catch { return iso }
}
</script>
