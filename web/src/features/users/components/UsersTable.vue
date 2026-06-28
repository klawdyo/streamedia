<template>

  <!-- Tabela de usuários: email, nome, roles badges, data, ações -->

  <div class="rounded-md border">

  <Table>

    <TableHeader>

    <TableRow>

      <TableHead>Email</TableHead>

      <TableHead>Nome</TableHead>

      <TableHead>Roles</TableHead>

      <TableHead>Level</TableHead>

      <TableHead>Data</TableHead>

      <TableHead class="text-right">Ações</TableHead>

    </TableRow>

    </TableHeader>

    <TableBody>

    <TableRow v-if="loading">

      <TableCell colspan="6" class="text-center py-8">

      <span class="text-muted-foreground">Carregando...</span>

      </TableCell>

    </TableRow>

    <TableRow v-else-if="!users.length">

      <TableCell colspan="6" class="text-center py-8">

      <span class="text-muted-foreground">Nenhum usuário encontrado.</span>

      </TableCell>

    </TableRow>

    <TableRow v-for="user in users" :key="user.id">

      <TableCell class="text-sm">

      <div class="flex items-center gap-2">

        <Avatar class="h-7 w-7">

        <AvatarImage v-if="user.picture" :src="user.picture" />

        <AvatarFallback class="text-xs">

          {{ getInitials(user.name) }}

        </AvatarFallback>

        </Avatar>

        <span class="truncate max-w-[200px]">{{ user.email }}</span>

      </div>

      </TableCell>

      <TableCell class="text-sm">{{ user.name || '—' }}</TableCell>

      <TableCell>

      <div class="flex flex-wrap gap-1">

        <Badge

        v-for="role in user.roles"

        :key="role.role"

        variant="outline"

        class="text-xs"

        >

        {{ role.role }}

        </Badge>

        <span v-if="!user.roles.length" class="text-xs text-muted-foreground">

        Sem roles

        </span>

      </div>

      </TableCell>

      <TableCell class="text-sm font-mono">

      {{ user.effective_level }}

      </TableCell>

      <TableCell class="text-sm">

      {{ formatDate(user.created_at) }}

      </TableCell>

      <TableCell class="text-right">

      <div class="flex items-center justify-end gap-1">

        <Button

        variant="ghost"

        size="icon-sm"

        title="Editar roles"

        @click="$emit('edit-roles', user)"

        >

        <PhGear :size="16" />

        </Button>

        <Button

        variant="ghost"

        size="icon-sm"

        title="Deletar"

        @click="$emit('delete', user.id)"

        >

        <PhTrash :size="16" class="text-destructive" />

        </Button>

      </div>

      </TableCell>

    </TableRow>

    </TableBody>

  </Table>

  </div>

</template>



<script setup lang="ts">

import { PhGear, PhTrash } from '@phosphor-icons/vue'

import {

  Table,

  TableBody,

  TableCell,

  TableHead,

  TableHeader,

  TableRow,

} from '@/components/ui/table'

import { Badge } from '@/components/ui/badge'

import { Button } from '@/components/ui/button'

import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar'

import type { UserWithRoles } from '@/types'



defineProps<{

  users: UserWithRoles[]

  loading: boolean

}>()



defineEmits<{

  'edit-roles': [user: UserWithRoles]

  delete: [userId: number]

}>()



function getInitials(name: string): string {

  if (!name) return '?'

  return name

  .split(' ')

  .map((n) => n.charAt(0))

  .join('')

  .toUpperCase()

  .slice(0, 2)

}



function formatDate(iso: string): string {

  if (!iso) return '—'

  return new Date(iso).toLocaleDateString('pt-BR')

}

</script>