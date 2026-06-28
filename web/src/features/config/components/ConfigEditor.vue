<template>
  <!-- Editor de item de configuração — renderiza input conforme o type -->
  <div class="space-y-3">
    <div class="flex items-start justify-between gap-2">
      <div class="flex-1">
        <div class="flex items-center gap-2">
          <code class="text-xs font-mono text-muted-foreground bg-muted px-1.5 py-0.5 rounded">
            {{ item.key }}
          </code>
          <Badge v-if="!item.visible" variant="secondary" class="text-xs">
            Write-only
          </Badge>
        </div>
        <p class="text-xs text-muted-foreground mt-1">
          {{ item.description }}
        </p>
        <p v-if="item.default !== undefined" class="text-xs text-muted-foreground">
          Default: {{ item.default }}
        </p>
      </div>
      <div class="shrink-0">
        <!-- Renderiza o input conforme o type -->
        <!-- string / url / secret → Input text/password/url -->
        <Input
          v-if="['string', 'url'].includes(item.type)"
          :type="item.type === 'url' ? 'url' : 'text'"
          :model-value="localValue"
          @update:model-value="localValue = String($event)"
          class="w-48 sm:w-64"
        />
        <Input
          v-else-if="item.type === 'secret'"
          type="password"
          :model-value="localValue"
          @update:model-value="localValue = String($event)"
          placeholder="••••••••"
          class="w-48 sm:w-64"
        />
        <!-- number / duration_seconds → Input number -->
        <Input
          v-else-if="['number', 'duration_seconds'].includes(item.type)"
          type="number"
          :model-value="localValue"
          @update:model-value="localValue = String($event)"
          class="w-32"
        />
        <!-- boolean → Toggle -->
        <button
          v-else-if="item.type === 'boolean'"
          role="switch"
          :aria-checked="localValue === 'true'"
          class="relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          :class="localValue === 'true' ? 'bg-primary' : 'bg-input'"
          @click="localValue = localValue === 'true' ? 'false' : 'true'"
        >
          <span
            class="pointer-events-none block h-4 w-4 rounded-full bg-background shadow-sm transition-transform"
            :class="localValue === 'true' ? 'translate-x-4' : 'translate-x-0.5'"
          />
        </button>
        <!-- Fallback: Input text -->
        <Input
          v-else
          type="text"
          :model-value="localValue"
          @update:model-value="localValue = String($event)"
          class="w-48 sm:w-64"
        />
      </div>
    </div>

    <!-- Validation info -->
    <p v-if="item.validation" class="text-xs text-muted-foreground">
      Validação: {{ item.validation }}
    </p>

    <!-- Botão salvar -->
    <div class="flex items-center gap-2">
      <Button
        size="sm"
        :disabled="!isDirty || saving"
        @click="handleSave"
      >
        <PhFloppyDisk :size="14" class="mr-1" />
        {{ saving ? 'Salvando...' : 'Salvar' }}
      </Button>
      <Button
        v-if="isDirty"
        variant="ghost"
        size="sm"
        @click="localValue = item.value ?? ''"
      >
        Cancelar
      </Button>
    </div>

    <!-- Mensagem de sucesso/erro -->
    <p v-if="saveMsg" class="text-xs" :class="saveError ? 'text-destructive' : 'text-green-600 dark:text-green-400'">
      {{ saveMsg }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { PhFloppyDisk } from '@phosphor-icons/vue'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import type { ConfigItem } from '@/types'
import { useConfigStore } from '@/features/config/stores/config'

const props = defineProps<{
  item: ConfigItem
}>()

const configStore = useConfigStore()
const localValue = ref(props.item.value ?? '')
const saving = ref(false)
const saveMsg = ref('')
const saveError = ref(false)

const isDirty = computed(() => localValue.value !== (props.item.value ?? ''))

// Reseta localValue quando o item muda (ex: após recarregar da API)
watch(() => props.item, (newItem) => {
  localValue.value = newItem.value ?? ''
}, { deep: true })

async function handleSave() {
  saving.value = true
  saveMsg.value = ''
  saveError.value = false

  const ok = await configStore.updateConfig(props.item.key, localValue.value)
  if (ok) {
    saveMsg.value = 'Salvo com sucesso.'
    saveError.value = false
  } else {
    saveMsg.value = configStore.error || 'Erro ao salvar.'
    saveError.value = true
  }
  saving.value = false

  // Limpa mensagem após 3s
  setTimeout(() => { saveMsg.value = '' }, 3000)
}
</script>
