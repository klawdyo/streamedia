<template>
  <!-- Player HLS com hls.js. Props: src, autoPlay. Emite eventos. Selector de resolução. -->
  <div class="relative w-full bg-black rounded-lg overflow-hidden">
    <video
      ref="videoRef"
      class="w-full aspect-video"
      controls
      :autoplay="autoPlay"
      playsinline
    />

    <!-- Selector de resolução (qualidade) -->
    <div
      v-if="levels.length > 1"
      class="absolute top-2 right-2 z-10"
    >
      <select
        class="rounded bg-black/60 text-white text-xs px-2 py-1 border border-white/20 focus:outline-none"
        :value="currentLevel"
        @change="setLevel(Number(($event.target as HTMLSelectElement).value))"
      >
        <option :value="-1">Auto</option>
        <option
          v-for="(level, idx) in levels"
          :key="idx"
          :value="idx"
        >
          {{ level.height }}p
        </option>
      </select>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'
import Hls, { Events as HlsEvents } from 'hls.js'
import type { Level } from 'hls.js'

const props = withDefaults(defineProps<{
  src: string
  autoPlay?: boolean
}>(), {
  autoPlay: false,
})

const emit = defineEmits<{
  playing: []
  paused: []
  ended: []
  error: [error: string]
}>()

const videoRef = ref<HTMLVideoElement>()
const levels = ref<Level[]>([])
const currentLevel = ref(-1)
let hls: Hls | null = null

function setLevel(idx: number) {
  if (hls) {
    hls.currentLevel = idx
    currentLevel.value = idx
  }
}

function initHls() {
  if (!videoRef.value) return
  const video = videoRef.value

  // Verifica suporte nativo a HLS (Safari)
  if (video.canPlayType('application/vnd.apple.mpegurl')) {
    video.src = props.src
    return
  }

  if (Hls.isSupported()) {
    hls = new Hls()
    hls.loadSource(props.src)
    hls.attachMedia(video)

    hls.on(HlsEvents.MANIFEST_PARSED, () => {
      levels.value = hls?.levels || []
    })

    hls.on(HlsEvents.LEVEL_SWITCHED, () => {
      currentLevel.value = hls?.currentLevel ?? -1
    })

    hls.on(HlsEvents.ERROR, (_event, data) => {
      if (data.fatal) {
        emit('error', data.type)
      }
    })
  } else {
    emit('error', 'HLS não suportado neste navegador')
  }
}

function setupVideoEvents() {
  if (!videoRef.value) return
  const video = videoRef.value

  video.addEventListener('playing', () => emit('playing'))
  video.addEventListener('pause', () => emit('paused'))
  video.addEventListener('ended', () => emit('ended'))
  video.addEventListener('error', () => emit('error', 'Erro ao reproduzir o vídeo'))
}

onMounted(() => {
  initHls()
  setupVideoEvents()
})

onUnmounted(() => {
  if (hls) {
    hls.destroy()
    hls = null
  }
})

// Reage a mudanças de src
watch(() => props.src, () => {
  if (hls) {
    hls.destroy()
    hls = null
  }
  if (videoRef.value) {
    videoRef.value.src = ''
  }
  initHls()
})
</script>
