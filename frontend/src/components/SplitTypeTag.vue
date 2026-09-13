<!-- SplitTypeTag：分摊方式标签 -->
<template>
  <StatusBadge :status="type" kind="split" />
</template>

<script setup lang="ts">
import StatusBadge from './StatusBadge.vue'

defineProps<{ type: string }>()
</script>
