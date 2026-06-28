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
        <!-- Campos secret: apenas botão "Alterar" que abre popup -->
        <Button
          v-if="item.type === 'secret'"
          variant="outline"
          size="sm"
          @click="openSecretDialog"
        >
          <PhPencilSimple :size="14" class="mr-1" />
          Alterar
        </Button>
        <!-- string / url → Input text/url -->
        <Input
          v-else-if="['string', 'url'].includes(item.type)"
          :type="item.type === 'url' ? 'url' : 'text'"
          :model-value="localValue"
          @update:model-value="localValue = String($event)"
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

    <!-- Botão salvar (não-secret) -->
    <div v-if="item.type !== 'secret'" class="flex items-center gap-2">
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

    <!-- Dialog para campos secret -->
    <Dialog v-model:open="showSecretDialog">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Alterar {{ item.key }}</DialogTitle>
          <DialogDescription>
            {{ item.description }}
          </DialogDescription>
        </DialogHeader>
        <div class="space-y-3 py-4">
          <Input
            :type="showSecretValue ? 'text' : 'password'"
            v-model="secretValue"
            :placeholder="'Novo valor para ' + item.key"
          />
          <div class="flex items-center gap-3">
            <Button
              v-if="item.key === 'webhook.secret'"
              variant="outline"
              size="sm"
              @click="generateToken"
            >
              <PhArrowsClockwise :size="14" class="mr-1" />
              Gerar Token
            </Button>
            <Button
              variant="ghost"
              size="sm"
              @click="showSecretValue = !showSecretValue"
            >
              <PhEye v-if="!showSecretValue" :size="14" class="mr-1" />
              <PhEyeClosed v-else :size="14" class="mr-1" />
              {{ showSecretValue ? 'Ocultar' : 'Mostrar' }}
            </Button>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" @click="showSecretDialog = false">
            Cancelar
          </Button>
          <Button @click="handleSaveSecret" :disabled="!secretValue.trim() || secretSaving">
            {{ secretSaving ? 'Salvando...' : 'Salvar' }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { PhFloppyDisk, PhPencilSimple, PhEye, PhEyeClosed, PhArrowsClockwise } from '@phosphor-icons/vue'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { ConfigItem } from '@/types'
import { useConfigStore } from '@/features/config/stores/config'
import { toast } from '@/composables/useToast'

const props = defineProps<{
  item: ConfigItem
}>()

const configStore = useConfigStore()
const localValue = ref(props.item.value ?? '')
const saving = ref(false)

// Dialog secret
const showSecretDialog = ref(false)
const showSecretValue = ref(false)
const secretValue = ref('')
const secretSaving = ref(false)

const isDirty = computed(() => localValue.value !== (props.item.value ?? ''))

// Reseta localValue quando o item muda (ex: após recarregar da API)
watch(() => props.item, (newItem) => {
  localValue.value = newItem.value ?? ''
}, { deep: true })

async function handleSave() {
  saving.value = true

  const ok = await configStore.updateConfig(props.item.key, localValue.value)
  if (ok) {
    toast.success(`"${props.item.key}" salvo com sucesso.`)
  } else {
    toast.error(configStore.error || 'Erro ao salvar.')
  }
  saving.value = false
}

function openSecretDialog() {
  secretValue.value = ''
  showSecretValue.value = false
  showSecretDialog.value = true
}

function generateToken() {
  const chars = 'abcdefghijklmnopqrstuvwxyz0123456789'
  let token = ''
  for (let i = 0; i < 16; i++) {
    token += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  secretValue.value = token
  showSecretValue.value = true
}

async function handleSaveSecret() {
  if (!secretValue.value.trim()) return
  secretSaving.value = true

  const ok = await configStore.updateConfig(props.item.key, secretValue.value.trim())
  if (ok) {
    toast.success(`"${props.item.key}" atualizado com sucesso.`)
    showSecretDialog.value = false
  } else {
    toast.error(configStore.error || 'Erro ao salvar.')
  }
  secretSaving.value = false
}
</script>
