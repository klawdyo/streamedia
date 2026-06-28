<template>
  <!-- Formulário de upload completo: tag, arquivo, progresso visual -->
  <div class="space-y-4">
    <div class="grid gap-3 sm:grid-cols-2">
      <div>
        <label class="text-xs font-medium text-muted-foreground mb-1 block">Tag do vídeo</label>
        <Input v-model="tag" placeholder="ex: video-teste-2024" />
      </div>
      <div>
        <label class="text-xs font-medium text-muted-foreground mb-1 block">Arquivo</label>
        <Input type="file" @change="onFileSelected" accept="video/*" />
      </div>
    </div>

    <!-- Info do arquivo selecionado -->
    <div v-if="selectedFile" class="flex items-center gap-3 rounded-md bg-muted px-3 py-2 text-sm">
      <PhFilmReel :size="16" class="text-muted-foreground" />
      <span class="flex-1 truncate">{{ selectedFile.name }}</span>
      <span class="text-muted-foreground text-xs">
        {{ formatBytes(selectedFile.size) }}
      </span>
    </div>

    <!-- Botão de upload -->
    <Button
      class="w-full"
      :disabled="!canUpload || uploadStore.currentUpload?.status === 'uploading'"
      @click="handleUpload"
    >
      <PhUpload v-if="uploadStore.currentUpload?.status !== 'uploading'" :size="16" class="mr-1" />
      <PhSpinnerGap v-else :size="16" class="mr-1 animate-spin" />
      {{ uploadButtonText }}
    </Button>

    <!-- Barra de progresso -->
    <div v-if="uploadStore.currentUpload" class="space-y-2">
      <div class="flex items-center justify-between text-sm">
        <span class="text-muted-foreground">
          {{ uploadStore.currentUpload.tag }}
        </span>
        <span class="font-medium">
          {{ uploadStore.currentUpload.status === 'done' ? 'Concluído' : `${uploadStore.currentUpload.progress}%` }}
        </span>
      </div>
      <div class="h-2 w-full overflow-hidden rounded-full bg-muted">
        <div
          class="h-full rounded-full transition-all duration-300"
          :class="progressColor"
          :style="{ width: `${uploadStore.currentUpload.progress}%` }"
        />
      </div>
      <p
        v-if="uploadStore.currentUpload.status === 'done' && uploadStore.currentUpload.video_id"
        class="text-xs text-green-600 dark:text-green-400"
      >
        Video ID: {{ uploadStore.currentUpload.video_id }}
      </p>
      <p
        v-if="uploadStore.currentUpload.status === 'error'"
        class="text-xs text-destructive"
      >
        {{ uploadStore.currentUpload.error }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { PhFilmReel, PhUpload, PhSpinnerGap } from '@phosphor-icons/vue'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { useUploadStore } from '../stores/upload'
import { toast } from '@/composables/useToast'

const emit = defineEmits<{
  uploaded: [videoId: string]
}>()

const uploadStore = useUploadStore()
const tag = ref('')
const selectedFile = ref<File | null>(null)

const canUpload = computed(() => tag.value.trim() && selectedFile.value)

const uploadButtonText = computed(() => {
  if (uploadStore.currentUpload?.status === 'uploading') return 'Enviando...'
  if (uploadStore.currentUpload?.status === 'done') return 'Enviar outro'
  return 'Iniciar Upload'
})

const progressColor = computed(() => {
  const status = uploadStore.currentUpload?.status
  if (status === 'done') return 'bg-green-500'
  if (status === 'error') return 'bg-destructive'
  return 'bg-primary'
})

function onFileSelected(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files && input.files.length > 0) {
    selectedFile.value = input.files[0]
  }
}

async function handleUpload() {
  if (!selectedFile.value || !tag.value.trim()) return

  const initData = await uploadStore.initUpload(
    tag.value.trim(),
    selectedFile.value.name,
    selectedFile.value.size,
  )

  if (!initData) {
    toast.error('Erro ao iniciar o upload. Verifique o tamanho e a tag.')
    return
  }

  const ok = await uploadStore.tusUpload(selectedFile.value, initData.location, initData.upload_id)
  if (ok && initData.video_id) {
    toast.success('Upload concluído com sucesso!')
    emit('uploaded', initData.video_id)
  } else if (!ok) {
    toast.error(uploadStore.currentUpload?.error || 'Erro durante o upload.')
  }
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
}
</script>
