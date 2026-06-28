<template>
  <!-- Gestão de usuários: tabela, criar, editar roles -->
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold tracking-tight">Usuários</h1>
        <p class="text-muted-foreground mt-1">
          Gerencie usuários e permissões
        </p>
      </div>
      <Button @click="showCreateDialog = true">
        <PhUserPlus :size="16" class="mr-1" />
        Novo Usuário
      </Button>
    </div>

    <!-- Tabela -->
    <UsersTable
      :users="store.users"
      :loading="store.loading"
      @edit-roles="openRolesDialog"
      @delete="handleDelete"
    />

    <span v-if="store.total > 0" class="text-sm text-muted-foreground">
      {{ store.total }} usuários no total
    </span>

    <!-- Dialog criar usuário -->
    <Dialog v-model:open="showCreateDialog">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Criar Usuário</DialogTitle>
          <DialogDescription>
            Adicione um novo usuário ao sistema
          </DialogDescription>
        </DialogHeader>
        <div class="space-y-3 py-4">
          <div>
            <label class="text-xs font-medium text-muted-foreground mb-1 block">Email</label>
            <Input v-model="newUserEmail" placeholder="usuario@exemplo.com" />
          </div>
          <p class="text-xs text-muted-foreground">
            O nome e a foto serão atualizados automaticamente quando o usuário fizer login.
          </p>
        </div>
        <DialogFooter>
          <Button variant="outline" @click="showCreateDialog = false">
            Cancelar
          </Button>
          <Button @click="handleCreate" :disabled="!newUserEmail.trim()">
            Criar
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Dialog editar roles -->
    <Dialog v-model:open="showRolesDialog">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Editar Roles</DialogTitle>
          <DialogDescription>
            {{ selectedUser?.email }}
          </DialogDescription>
        </DialogHeader>
        <div class="py-4">
          <RolesSelect
            v-if="selectedUser"
            :available-roles="availableRoles"
            v-model:current-roles="editingRoles"
            :effective-level="auth.effectiveLevel"
          />
        </div>
        <DialogFooter>
          <Button variant="outline" @click="showRolesDialog = false">
            Cancelar
          </Button>
          <Button @click="handleSaveRoles">
            Salvar
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Erro -->
    <div v-if="store.error" class="rounded-md bg-destructive/10 p-4 text-sm text-destructive">
      {{ store.error }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { PhUserPlus } from '@phosphor-icons/vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useUsersStore } from '../stores/users'
import { useAuthStore } from '@/stores/auth'
import type { UserWithRoles, UserRole } from '@/types'
import UsersTable from '../components/UsersTable.vue'
import RolesSelect from '../components/RolesSelect.vue'
import { toast } from '@/composables/useToast'

const store = useUsersStore()
const auth = useAuthStore()

// Dialog criar usuário
const showCreateDialog = ref(false)
const newUserEmail = ref('')

// Dialog roles
const showRolesDialog = ref(false)
const selectedUser = ref<UserWithRoles | null>(null)
const editingRoles = ref<string[]>([])

// Roles disponíveis (hardcoded como fallback, idealmente viria da API)
const availableRoles: UserRole[] = [
  { role: 'dev', level_num: 200 },
  { role: 'admin', level_num: 100 },
  { role: 'acl', level_num: 50 },
  { role: 'user', level_num: 10 },
]

function openRolesDialog(user: UserWithRoles) {
  selectedUser.value = user
  editingRoles.value = user.roles.map((r) => r.role)
  showRolesDialog.value = true
}

async function handleCreate() {
  const user = await store.createUser(newUserEmail.value.trim())
  if (user) {
    toast.success('Usuário criado com sucesso.')
    showCreateDialog.value = false
    newUserEmail.value = ''
  } else {
    toast.error(store.error || 'Erro ao criar usuário.')
  }
}

async function handleSaveRoles() {
  if (!selectedUser.value) return
  const updated = await store.updateRoles(selectedUser.value.id, editingRoles.value)
  if (updated) {
    toast.success('Permissões atualizadas com sucesso.')
    showRolesDialog.value = false
  } else {
    toast.error(store.error || 'Erro ao atualizar permissões.')
  }
}

async function handleDelete(userId: number) {
  if (!confirm('Tem certeza que deseja deletar este usuário?')) return
  const ok = await store.deleteUser(userId)
  if (ok) {
    toast.success('Usuário deletado com sucesso.')
  } else {
    toast.error(store.error || 'Erro ao deletar usuário.')
  }
}

onMounted(() => {
  store.fetchUsers()
})
</script>
