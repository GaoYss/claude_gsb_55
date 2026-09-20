<template>
  <el-drawer
    :model-value="modelValue"
    title="修订记录"
    size="620px"
    @update:model-value="$emit('update:modelValue', $event)"
    @open="load"
  >
    <div v-loading="loading">
      <el-alert v-if="repair" type="info" :closable="false" class="drawer-block">
        维修单 {{ repair.repair_no }} 原始登记: 结果
        <StatusTag :dict="REPAIR_RESULT" :value="repair.result" />
        、金额 {{ formatMoney(repair.cost) }} 、完工时间 {{ formatDateTime(repair.finished_at) }}。
        原始记录不可改写, 以下为全部更正轨迹。
      </el-alert>

      <el-empty v-if="!revisions.length" description="暂无修订记录" />

      <el-timeline v-else class="drawer-block">
        <el-timeline-item
          v-for="item in revisions"
          :key="item.id"
          :timestamp="formatDateTime(item.created_at)"
          placement="top"
          :type="item.cross_settled ? 'danger' : 'primary'"
        >
          <el-card shadow="never">
            <div class="revision-head">
              <span class="revision-no">{{ item.revision_no }}</span>
              <el-tag size="small" effect="plain">第 {{ item.seq }} 次修订</el-tag>
              <el-tag v-if="item.cross_settled" size="small" type="danger" effect="plain">
                跨月调整 → {{ item.effective_month }}
              </el-tag>
            </div>

            <el-table :data="item.changes" size="small" border class="change-table">
              <el-table-column prop="label" label="字段" width="100" />
              <el-table-column label="更正前" min-width="110">
                <template #default="{ row }">{{ row.old_value }}</template>
              </el-table-column>
              <el-table-column label="更正后" min-width="110">
                <template #default="{ row }">{{ row.new_value }}</template>
              </el-table-column>
            </el-table>

            <div class="revision-meta text-muted">
              批次 {{ item.batch_no }} · 操作人 {{ item.operator }} · 原因: {{ item.reason }}
            </div>
          </el-card>
        </el-timeline-item>
      </el-timeline>
    </div>
  </el-drawer>
</template>

<script setup>
import { ref } from 'vue'
import StatusTag from '@/components/common/StatusTag.vue'
import { repairApi } from '@/api/repair'
import { REPAIR_RESULT } from '@/constants/dict'
import { formatDateTime, formatMoney } from '@/utils/format'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  repair: { type: Object, default: null },
})

defineEmits(['update:modelValue'])

const loading = ref(false)
const revisions = ref([])

async function load() {
  if (!props.repair) return
  loading.value = true
  try {
    revisions.value = await repairApi.repairRevisions(props.repair.id)
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.drawer-block {
  margin-bottom: 16px;
}

.revision-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
}

.revision-no {
  font-weight: 600;
}

.change-table {
  margin-bottom: 10px;
}

.revision-meta {
  font-size: 12px;
  line-height: 1.6;
}
</style>
