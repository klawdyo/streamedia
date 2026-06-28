<template>
  <!-- Multi-select de roles com regra de desabilitação por nível hierárquico -->
  <div class="space-y-2">
    <label class="text-xs font-medium text-muted-foreground block">
      Roles
    </label>
    <div class="grid gap-2">
      <div
        v-for="role in availableRoles"
        :key="role.role"
        class="flex items-center gap-2 rounded-md border px-3 py-2 text-sm"
        :class="{
          'opacity-50 cursor-not-allowed': isDisabled(role.role),
          'cursor-pointer hover:bg-accent': !isDisabled(role.role),
          'bg-accent': isSelected(role.role),
        }"
        @click="toggleRole(role.role)"
      >
        <div
          class="flex h-4 w-4 shrink-0 items-center justify-center rounded-sm border"
          :class="{
            'bg-primary border-primary text-primary-foreground': isSelected(role.role),
            'border-muted-foreground/30': !isSelected(role.role),
          }"
        >
          <PhCheck v-if="isSelected(role.role)" :size="12" />
        </div>
        <span class="flex-1">{{ role.role }}</span>
        <Badge variant="outline" class="text-xs font-mono">
          Level {{ role.level_num }}
        </Badge>
      </div>
    </div>

    <!-- Info -->
    <p class="text-xs text-muted-foreground">
      Roles com level acima do seu nível efetivo não podem ser alteradas.
    </p>
  </div>
</template>

<script setup lang="ts">
import { PhCheck } from '@phosphor-icons/vue'
import { Badge } from '@/components/ui/badge'
import type { UserRole } from '@/types'

const props = defineProps<{
  availableRoles: UserRole[]
  currentRoles: string[]
  effectiveLevel: number
}>()

const emit = defineEmits<{
  'update:currentRoles': [roles: string[]]
}>()

function isSelected(role: string): boolean {
  return props.currentRoles.includes(role)
}

function isDisabled(role: string): boolean {
  const roleObj = props.availableRoles.find((r) => r.role === role)
  if (!roleObj) return false
  // Regra: se o level da role for maior que o nível efetivo → desabilitado
  return roleObj.level_num > props.effectiveLevel
}

function toggleRole(role: string) {
  if (isDisabled(role)) return

  const newRoles = isSelected(role)
    ? props.currentRoles.filter((r) => r !== role)
    : [...props.currentRoles, role]

  emit('update:currentRoles', newRoles)
}
</script>
