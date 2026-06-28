<template>

  <!-- Playground interativo da API Streamedia — Modos "Docs" e "Try" -->

  <div class="space-y-6">

  <div>

    <h1 class="text-2xl font-bold tracking-tight">API Playground</h1>

    <p class="text-muted-foreground mt-1">

    Documentação interativa e testador de endpoints da API Streamedia

    </p>

  </div>



  <!-- Tabs: Docs / Try -->

  <div class="flex items-center gap-1 rounded-lg bg-muted p-1 w-fit">

    <button

    class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"

    :class="mode === 'docs' ? 'bg-background shadow-sm' : 'text-muted-foreground hover:text-foreground'"

    @click="mode = 'docs'"

    >

    <PhBookOpen :size="16" class="inline mr-1" />

    Docs

    </button>

    <button

    class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"

    :class="mode === 'try' ? 'bg-background shadow-sm' : 'text-muted-foreground hover:text-foreground'"

    @click="mode = 'try'"

    >

    <PhPlay :size="16" class="inline mr-1" />

    Try

    </button>

  </div>



  <!-- Modo Docs: documentação dos endpoints -->

  <div v-if="mode === 'docs'" class="space-y-6">

    <!-- Upload section -->

    <Card>

    <CardHeader>

      <CardTitle class="flex items-center gap-2">

      <PhUpload :size="20" />

      Upload de Vídeo

      </CardTitle>

      <CardDescription>

      Fluxo completo de upload via TUS: init → upload → play → SSE

      </CardDescription>

    </CardHeader>

    <CardContent class="space-y-6">

      <!-- Upload Form inline -->

      <UploadForm @uploaded="onUploaded" />

    </CardContent>

    </Card>



    <!-- API Endpoints reference -->

    <div class="space-y-4">

    <h2 class="text-lg font-semibold">Endpoints da API</h2>

    <Card v-for="ep in endpoints" :key="ep.path">

      <CardHeader>

      <div class="flex items-center gap-3">

        <Badge :variant="methodVariant(ep.method)" class="font-mono text-xs">

        {{ ep.method }}

        </Badge>

        <code class="text-sm font-mono text-foreground">{{ ep.path }}</code>

      </div>

      <CardDescription>{{ ep.description }}</CardDescription>

      </CardHeader>

      <CardContent class="space-y-4">

      <!-- Headers -->

      <div>

        <h4 class="text-xs font-semibold text-muted-foreground mb-2">Headers</h4>

        <div class="space-y-1">

        <div v-for="h in ep.headers" :key="h.key" class="flex gap-2 text-sm">

          <code class="text-xs font-mono text-muted-foreground bg-muted px-1 rounded">{{ h.key }}:</code>

          <span class="text-xs">{{ h.value }}</span>

        </div>

        </div>

      </div>

      <!-- Body -->

      <div v-if="ep.body">

        <h4 class="text-xs font-semibold text-muted-foreground mb-2">Body</h4>

        <pre class="rounded-md bg-muted p-3 text-xs font-mono overflow-auto">{{ JSON.stringify(ep.body, null, 2) }}</pre>

      </div>

      <!-- Responses -->

      <div v-if="ep.responses.length">

        <h4 class="text-xs font-semibold text-muted-foreground mb-2">Respostas</h4>

        <div v-for="(r, i) in ep.responses" :key="i" class="mb-2">

        <Badge variant="outline" class="text-xs mb-1">{{ r.status }}</Badge>

        <pre class="rounded-md bg-muted p-3 text-xs font-mono overflow-auto">{{ JSON.stringify(r.body, null, 2) }}</pre>

        </div>

      </div>

      </CardContent>

    </Card>

    </div>

  </div>



  <!-- Modo Try: formulário + resposta + SSE log -->

  <div v-else class="space-y-6">

    <!-- Seletor de endpoint -->

    <Card>

    <CardHeader>

      <CardTitle>Testar Endpoint</CardTitle>

      <CardDescription>Preencha os parâmetros e execute a requisição</CardDescription>

    </CardHeader>

    <CardContent class="space-y-4">

      <div class="flex items-center gap-3 flex-wrap">

      <Select v-model="selectedEndpoint">

        <SelectTrigger class="w-64">

        <SelectValue placeholder="Selecione um endpoint" />

        </SelectTrigger>

        <SelectContent>

        <SelectItem v-for="ep in tryEndpoints" :key="ep.path" :value="JSON.stringify(ep)">

          <span class="font-mono text-xs">{{ ep.method }}</span>

          <span class="ml-2">{{ ep.path }}</span>

        </SelectItem>

        </SelectContent>

      </Select>



      <Button @click="executeRequest" :disabled="!selectedEndpoint || executing">

        <PhPlay :size="16" class="mr-1" />

        {{ executing ? 'Executando...' : 'Executar' }}

      </Button>

      </div>



      <!-- Campos dinâmicos do body -->

      <div v-if="parsedEndpoint" class="space-y-3">

      <template v-if="parsedEndpoint.body">

        <div v-for="(val, key) in parsedEndpoint.body" :key="key">

        <label class="text-xs font-medium text-muted-foreground mb-1 block">{{ key }}</label>

        <Input v-model="requestBody[key]" :placeholder="String(val)" />

        </div>

      </template>

      </div>



      <!-- Resposta -->

      <div v-if="responseText" class="space-y-2">

      <div class="flex items-center gap-2">

        <h4 class="text-xs font-semibold">Resposta</h4>

        <Badge variant="outline" class="text-xs font-mono">{{ responseStatus }}</Badge>

      </div>

      <pre class="rounded-md bg-muted p-3 text-xs font-mono overflow-auto max-h-64">{{ responseText }}</pre>

      </div>

    </CardContent>

    </Card>



    <!-- SSE Log -->

    <Card>

    <CardHeader>

      <CardTitle class="flex items-center gap-2">

      <PhBroadcast :size="20" />

      Log SSE

      </CardTitle>

      <CardDescription>Eventos em tempo real do último upload</CardDescription>

    </CardHeader>

    <CardContent>

      <SSELog :events="sseLogEvents" />

    </CardContent>

    </Card>

  </div>

  </div>

</template>



<script setup lang="ts">

import { ref, computed } from 'vue'

import { PhBookOpen, PhPlay, PhUpload, PhBroadcast } from '@phosphor-icons/vue'

import { Button } from '@/components/ui/button'

import { Badge } from '@/components/ui/badge'

import { Input } from '@/components/ui/input'

import {

  Card,

  CardContent,

  CardDescription,

  CardHeader,

  CardTitle,

} from '@/components/ui/card'

import {

  Select,

  SelectContent,

  SelectItem,

  SelectTrigger,

  SelectValue,

} from '@/components/ui/select'

import { api } from '@/api/client'

import type { SSEEvent } from '@/types'

import UploadForm from '../components/UploadForm.vue'

import SSELog from '../components/SSELog.vue'



// =========== Modo ===========

const mode = ref<'docs' | 'try'>('docs')



// =========== Endpoints documentados ===========

interface EndpointDoc {

  method: string

  path: string

  description: string

  headers: { key: string; value: string }[]

  body?: Record<string, unknown>

  responses: { status: number; body: Record<string, unknown> }[]

}



const endpoints: EndpointDoc[] = [

  {

  method: 'POST',

  path: '/api/upload/init',

  description: 'Inicia um upload de vídeo. Retorna um upload_id e uma location TUS.',

  headers: [

    { key: 'Authorization', value: 'Bearer <token>' },

    { key: 'Content-Type', value: 'application/json' },

    { key: 'X-Streamedia-Csrf', value: '1' },

  ],

  body: { tag: 'meu-video', filename: 'video.mp4', size: 1024000 },

  responses: [

    { status: 200, body: { data: { upload_id: 'abc123', location: '/api/upload/abc123', video_id: 'vid_xyz' } } },

    { status: 400, body: { error: 'tag é obrigatório' } },

  ],

  },

  {

  method: 'PATCH',

  path: '/api/upload/{upload_id}',

  description: 'Upload TUS resumable. Envia o arquivo em chunks com cabeçalhos TUS.',

  headers: [

    { key: 'Content-Type', value: 'application/offset+octet-stream' },

    { key: 'Upload-Offset', value: '0' },

    { key: 'Tus-Resumable', value: '1.0.0' },

  ],

  body: undefined,

  responses: [

    { status: 204, body: { data: { ok: true } } },

  ],

  },

  {

  method: 'POST',

  path: '/api/play/init',

  description: 'Inicia a reprodução de um vídeo. Retorna a URL HLS.',

  headers: [

    { key: 'Authorization', value: 'Bearer <token>' },

    { key: 'Content-Type', value: 'application/json' },

    { key: 'X-Streamedia-Csrf', value: '1' },

  ],

  body: { video_id: 'vid_xyz', token: '<token>' },

  responses: [

    { status: 200, body: { data: { hls_url: '/api/hls/vid_xyz/master.m3u8', video_id: 'vid_xyz', status: 'ready' } } },

    { status: 404, body: { error: 'vídeo não encontrado' } },

  ],

  },

  {

  method: 'GET',

  path: '/api/status/{video_id}',

  description: 'Consulta o status atual de um vídeo.',

  headers: [

    { key: 'Authorization', value: 'Bearer <token>' },

  ],

  body: undefined,

  responses: [

    { status: 200, body: { data: { video_id: 'vid_xyz', status: 'ready', tag: 'meu-video', duration_s: 120, actual_size_bytes: 5242880 } } },

  ],

  },

  {

  method: 'GET',

  path: '/api/events?video_id=X&token=Y',

  description: 'EventSource SSE para eventos em tempo real de um vídeo.',

  headers: [],

  body: undefined,

  responses: [

    { status: 200, body: { event: 'status', video_id: 'vid_xyz', status: 'processing', timestamp: '2024-01-01T00:00:00Z' } },

  ],

  },

  {

  method: 'GET',

  path: '/api/auth/me',

  description: 'Retorna os dados do usuário autenticado e suas roles.',

  headers: [

    { key: 'Authorization', value: 'Bearer <token>' },

  ],

  body: undefined,

  responses: [

    { status: 200, body: { data: { email: 'user@exemplo.com', name: 'Usuário', picture: 'https://...', roles: [{ role: 'admin', level_num: 100 }], effective_level: 100 } } },

    { status: 401, body: { error: 'não autenticado' } },

  ],

  },

]



// Endpoints que podem ser testados via formulário (métodos não-GET ignorados no Try)

const tryEndpoints = endpoints.filter((ep) =>

  ['POST', 'PUT', 'DELETE'].includes(ep.method)

)



// =========== Modo Try ===========

const selectedEndpoint = ref('')

const requestBody = ref<Record<string, string>>({})

const responseText = ref('')

const responseStatus = ref('')

const executing = ref(false)



const parsedEndpoint = computed(() => {

  if (!selectedEndpoint.value) return null

  try {

  return JSON.parse(selectedEndpoint.value) as EndpointDoc

  } catch {

  return null

  }

})



async function executeRequest() {

  const ep = parsedEndpoint.value

  if (!ep) return



  executing.value = true

  responseText.value = ''

  responseStatus.value = ''



  try {

  let res: { data?: unknown; error?: string; meta?: unknown }

  const body = Object.keys(requestBody.value).length > 0 ? requestBody.value : undefined



  switch (ep.method) {

    case 'POST':

    res = await api.post(ep.path, body)

    break

    case 'PUT':

    res = await api.put(ep.path, body)

    break

    case 'DELETE':

    res = await api.del(ep.path)

    break

    default:

    res = { error: 'Método não suportado no Try' }

  }



  responseStatus.value = res.error ? 'Erro' : '200 OK'

  responseText.value = JSON.stringify(res, null, 2)

  } catch (e) {

  responseStatus.value = 'Erro'

  responseText.value = String(e)

  } finally {

  executing.value = false

  }

}



// =========== SSE Log ===========

const sseLogEvents = ref<SSEEvent[]>([])



function onUploaded(videoId: string) {

  sseLogEvents.value.push({

  event: 'uploaded',

  video_id: videoId,

  timestamp: new Date().toISOString(),

  })

}



function methodVariant(method: string): 'default' | 'secondary' | 'destructive' | 'outline' {

  switch (method) {

  case 'GET': return 'default'

  case 'POST': return 'secondary'

  case 'PATCH': return 'outline'

  case 'DELETE': return 'destructive'

  default: return 'outline'

  }

}

</script>