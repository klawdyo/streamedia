<template>

  <!-- Toggle de tema dark/light com ícone phosphor -->

  <button

  class="relative inline-flex items-center justify-center rounded-md p-2 text-foreground hover:bg-accent hover:text-accent-foreground transition-colors"

  @click="theme.toggle()"

  :title="theme.isDark.value ? 'Modo claro' : 'Modo escuro'"

  >

  <!-- Ícone solar (claro) — visível em dark mode -->

  <PhSunHorizon

    v-if="theme.isDark.value"

    :size="20"

    weight="fill"

  />

  <!-- Ícone lunar (escuro) — visível em light mode -->

  <PhMoonStars

    v-else

    :size="20"

    weight="fill"

  />

  </button>

</template>



<script setup lang="ts">

import { PhSunHorizon, PhMoonStars } from '@phosphor-icons/vue'

import { useTheme } from '@/composables/useTheme'



const theme = useTheme()

</script>