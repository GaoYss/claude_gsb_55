<template>
  <span class="repair-result">
    <StatusTag v-if="value" :dict="REPAIR_RESULT" :value="value" />
    <span v-else class="text-muted">-</span>
    <el-tooltip
      v-if="corrected"
      :content="`原结果: ${dictLabel(REPAIR_RESULT, row.result)}, 已更正 ${row.correction_count} 次`"
      placement="top"
    >
      <el-tag size="small" type="warning" effect="plain" class="corrected-tag">已更正</el-tag>
    </el-tooltip>
  </span>
</template>

<script setup>
import { computed } from 'vue'
import StatusTag from '@/components/common/StatusTag.vue'
import { REPAIR_RESULT, dictLabel } from '@/constants/dict'

// 维修结果展示: 一律显示当前生效结果, 更正过的记录附带"已更正"标记, 原始结果在提示中查看。
const props = defineProps({
  row: { type: Object, required: true },
})

const value = computed(() => props.row.current_result || props.row.result)
const corrected = computed(() => (props.row.correction_count ?? 0) > 0)
</script>

<style scoped>
.repair-result {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.corrected-tag {
  cursor: default;
}
</style>
