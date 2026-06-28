<template>

  <!-- Lista de vídeos com filtros, tabela paginada e ações -->

  <div class="space-y-6">

  <div class="flex items-center justify-between">

    <div>

    <h1 class="text-2xl font-bold tracking-tight">Vídeos</h1>

    <p class="text-muted-foreground mt-1">

      Gerencie os vídeos do servidor

    </p>

    </div>

    <Button @click="goToPlayground">

    <PhPlus :size="16" class="mr-1" />

    Novo Upload

    </Button>

  </div>



  <!-- Filtros -->

  <Card>

    <CardContent class="pt-6">

    <div class="flex flex-wrap gap-3">

      <div class="w-40">

      <label class="text-xs font-medium text-muted-foreground mb-1 block">Status</label>

      <Select v-model="statusFilter">

        <SelectTrigger>

        <SelectValue placeholder="Todos" />

        </SelectTrigger>

        <SelectContent>

        <SelectItem value="all">Todos</SelectItem>

        <SelectItem value="ready">Pronto</SelectItem>

        <SelectItem value="processing">Processando</SelectItem>

        <SelectItem value="uploading">Enviando</SelectItem>

        <SelectItem value="failed">Falhou</SelectItem>

        <SelectItem value="deleted">Deletado</SelectItem>

        </SelectContent>

      </Select>

      </div>

      <div class="w-48">

      <label class="text-xs font-medium text-muted-foreground mb-1 block">Tag</label>

      <Input v-model="tagFilter" placeholder="Filtrar por tag..." />

      </div>

      <div class="w-40">

      <label class="text-xs font-medium text-muted-foreground mb-1 block">Ordenar</label>

      <Select v-model="sortFilter">

        <SelectTrigger>

        <SelectValue placeholder="Recente" />

        </SelectTrigger>

        <SelectContent>

        <SelectItem value="newest">Mais recente</SelectItem>

        <SelectItem value="oldest">Mais antigo</SelectItem>

        <SelectItem value="largest">Maior tamanho</SelectItem>

        <SelectItem value="smallest">Menor tamanho</SelectItem>

        </SelectContent>

      </Select>

      </div>

      <div class="flex items-end">

      <Button variant="secondary" @click="applyFilters">

        <PhFunnel :size="16" class="mr-1" />

        Filtrar

      </Button>

      </div>

    </div>

    </CardContent>

  </Card>



  <!-- Tabela -->

  <VideoTable

    :videos="store.videos"

    :loading="store.loading"

    @view="goToVideo"

    @reprocess="handleReprocess"

    @delete="handleDelete"

  />



  <!-- Paginação -->

  <div v-if="store.totalPages > 1" class="flex items-center justify-between">

    <span class="text-sm text-muted-foreground">

    {{ store.total }} vídeos no total

    </span>

    <div class="flex items-center gap-2">

    <Button

      variant="outline"

      size="sm"

      :disabled="store.page <= 1"

      @click="goToPage(store.page - 1)"

    >

      Anterior

    </Button>

    <span class="text-sm text-muted-foreground">

      Página {{ store.page }} de {{ store.totalPages }}

    </span>

    <Button

      variant="outline"

      size="sm"

      :disabled="store.page >= store.totalPages"

      @click="goToPage(store.page + 1)"

    >

      Próxima

    </Button>

    </div>

  </div>

  </div>

</template>



<script setup lang="ts">

import { ref, onMounted } from 'vue'

import { useRouter } from 'vue-router'

import { PhPlus, PhFunnel } from '@phosphor-icons/vue'

import { Button } from '@/components/ui/button'

import { Input } from '@/components/ui/input'

import { Card, CardContent } from '@/components/ui/card'

import {

  Select,

  SelectContent,

  SelectItem,

  SelectTrigger,

  SelectValue,

} from '@/components/ui/select'

import { useVideosStore } from '../stores/videos'

import VideoTable from '../components/VideoTable.vue'

import { toast } from '@/composables/useToast'



const router = useRouter()

const store = useVideosStore()



const statusFilter = ref('all')

const tagFilter = ref('')

const sortFilter = ref('newest')



function applyFilters() {

  store.setFilters({

  status: statusFilter.value === 'all' ? undefined : statusFilter.value,

  tag: tagFilter.value || undefined,

  sort: sortFilter.value,

  page: 1,

  })

  store.fetchVideos()

}



function goToPage(p: number) {

  store.setFilters({ page: p })

  store.fetchVideos()

}



function goToVideo(videoId: string) {

  router.push({ name: 'video-detail', params: { id: videoId } })

}



function goToPlayground() {

  router.push({ name: 'playground' })

}



async function handleReprocess(videoId: string) {

  const ok = await store.reprocessVideo(videoId)

  if (ok) {

  toast.success('Vídeo enviado para reprocessamento.')

  store.fetchVideos()

  } else {

  toast.error(store.error || 'Erro ao reprocessar vídeo.')

  }

}



async function handleDelete(videoId: string) {

  if (!confirm('Tem certeza que deseja deletar este vídeo?')) return

  const ok = await store.deleteVideo(videoId)

  if (ok) {

  toast.success('Vídeo deletado com sucesso.')

  store.fetchVideos()

  } else {

  toast.error(store.error || 'Erro ao deletar vídeo.')

  }

}



onMounted(() => {

  store.fetchVideos()

})

</script>