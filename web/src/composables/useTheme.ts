// Composable de tema dark/light



import { ref, watchEffect } from 'vue'



const isDark = ref(false)



export function useTheme() {

  // Inicializa a partir do localStorage ou preferência do sistema

  const stored = localStorage.getItem('theme')

  if (stored === 'dark' || (!stored && window.matchMedia('(prefers-color-scheme: dark)').matches)) {

  isDark.value = true

  }



  // Sincroniza classe 'dark' no <html>

  watchEffect(() => {

  const root = document.documentElement

  if (isDark.value) {

    root.classList.add('dark')

  } else {

    root.classList.remove('dark')

  }

  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')

  })



  function toggle() {

  isDark.value = !isDark.value

  }



  return {

  isDark,

  toggle,

  }

}