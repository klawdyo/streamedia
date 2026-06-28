<template>
  <!-- Página de configurações dinâmicas agrupadas por group_key -->
  <div class="space-y-6">
    <div>
      <h1 class="text-2xl font-bold tracking-tight">Configurações</h1>
      <p class="text-muted-foreground mt-1">
        Gerencie as configurações dinâmicas do servidor
      </p>
    </div>

    <div v-if="configStore.loading" class="space-y-4">
      <Skeleton v-for="i in 3" :key="i" class="h-32 w-full rounded-lg" />
    </div>

    <div v-else-if="configStore.error" class="rounded-md bg-destructive/10 p-4 text-sm text-destructive">
      {{ configStore.error }}
    </div>

    <!-- Grupos de configuração -->
    <div v-else class="space-y-6">
      <Card v-for="group in configStore.groups" :key="group.key">
        <CardHeader>
          <CardTitle>{{ group.title }}</CardTitle>
          <CardDescription v-if="group.description">
            {{ group.description }}
          </CardDescription>
        </CardHeader>
        <CardContent class="space-y-6">
          <div v-if="!group.items.length" class="text-sm text-muted-foreground">
            Nenhuma configuração neste grupo.
          </div>
          <template v-for="(item, idx) in group.items" :key="item.key">
            <Separator v-if="idx > 0" />
            <ConfigEditor :item="item" />
          </template>
        </CardContent>
      </Card>

      <!-- Estado vazio -->
      <div v-if="!configStore.groups.length" class="text-center py-12 text-muted-foreground">
        <PhGear :size="48" class="mx-auto mb-3 opacity-30" />
        <p class="text-sm">Nenhuma configuração encontrada.</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { PhGear } from '@phosphor-icons/vue'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { useConfigStore } from '../stores/config'
import ConfigEditor from '../components/ConfigEditor.vue'

const configStore = useConfigStore()

onMounted(() => {
  configStore.fetchConfig()
})
</script>
