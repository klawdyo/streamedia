<template>
  <!-- Container fixo de toasts — empilhados no canto inferior direito -->
  <Teleport to="body">
    <div
      v-if="t.length > 0"
      class="fixed bottom-4 right-4 z-50 flex flex-col-reverse gap-2"
    >
      <div
        v-for="toast in t"
        :key="toast.id"
        role="alert"
        class="pointer-events-auto flex items-center gap-2 rounded-lg border px-4 py-3 text-sm shadow-lg transition-all animate-in slide-in-from-right"
        :class="{
          'bg-green-50 border-green-200 text-green-800 dark:bg-green-950 dark:border-green-800 dark:text-green-200': toast.type === 'success',
          'bg-destructive/10 border-destructive/30 text-destructive': toast.type === 'error',
          'bg-muted border-border': toast.type === 'info',
        }"
      >
        <PhCheckCircle v-if="toast.type === 'success'" :size="18" class="text-green-600 dark:text-green-400 shrink-0" />
        <PhXCircle v-else-if="toast.type === 'error'" :size="18" class="text-destructive shrink-0" />
        <PhInfo v-else :size="18" class="text-muted-foreground shrink-0" />
        <span>{{ toast.message }}</span>
        <button
          class="ml-auto shrink-0 rounded-md p-0.5 hover:bg-black/5 dark:hover:bg-white/10"
          @click="dismiss(toast.id)"
        >
          <PhX :size="14" />
        </button>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { PhCheckCircle, PhXCircle, PhInfo, PhX } from '@phosphor-icons/vue'
import { toast } from '@/composables/useToast'

const { toasts: t, dismiss } = toast
</script>
