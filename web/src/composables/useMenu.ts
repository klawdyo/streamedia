// Composable que computa o menu lateral a partir do router + permissões do usuário.
// Agrupa itens por `parent` e ordena por `order`.

import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

export interface MenuItem {
  title: string
  icon: string
  to: { name: string }
  order: number
}

export interface MenuGroup {
  key: string
  items: MenuItem[]
}

export function useMenu() {
  const router = useRouter()
  const auth = useAuthStore()

  const menu = computed(() => {
    const groups: Record<string, MenuItem[]> = {}
    const ungrouped: MenuItem[] = []

    for (const route of router.getRoutes()) {
      const meta = route.meta
      if (!meta.showInMenu) continue
      // Verifica permissão
      if (meta.permissions.length > 0 && !auth.canAccess(meta.permissions)) continue

      const item: MenuItem = {
        title: meta.title as string,
        icon: (meta.icon as string) || '',
        to: { name: route.name as string },
        order: (meta.order as number) || 99,
      }

      if (meta.parent) {
        const parent = meta.parent as string
        if (!groups[parent]) groups[parent] = []
        groups[parent].push(item)
      } else {
        ungrouped.push(item)
      }
    }

    // Ordena cada grupo por `order`
    const result: MenuGroup[] = []

    for (const [key, items] of Object.entries(groups)) {
      items.sort((a, b) => a.order - b.order)
      result.push({ key, items })
    }

    // Itens sem grupo ficam no topo
    ungrouped.sort((a, b) => a.order - b.order)
    if (ungrouped.length > 0) {
      result.unshift({ key: '__root__', items: ungrouped })
    }

    return result
  })

  return { menu }
}
