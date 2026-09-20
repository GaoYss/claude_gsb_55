<template>
  <el-drawer
    :model-value="modelValue"
    :title="repair ? `修订路径 - ${repair.repair_no}` : '结果更正记录'"
    size="720px"
    @update:model-value="$emit('update:modelValue', $event)"
    @open="load"
  >
    <div class="drawer-body">
      <el-alert v-if="repair" type="info" :closable="false" class="path-hint">
        当前生效结果以最新一条修订为准; 原始结果与完工时间保持完工时的取值, 不会被改写。
      </el-alert>

      <el-table v-loading="loading" :data="rows" size="small" border row-key="id" :expand-row-keys="expanded">
        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="changes-panel">
              <div class="changes-title">更正前后逐字段对照</div>
              <el-table :data="row.changes" size="small" border>
                <el-table-column prop="field_label" label="字段" width="110" />
                <el-table-column label="更正前" width="140">
                  <template #default="{ row: change }">
                    <StatusTag :dict="dictOf(change.field)" :value="change.before" />
                  </template>
                </el-table-column>
                <el-table-column label="更正后" width="140">
                  <template #default="{ row: change }">
                    <StatusTag :dict="dictOf(change.field)" :value="change.after" />
                  </template>
                </el-table-column>
                <el-table-column label="原始值">
                  <template #default="{ row: change }">
                    <span class="text-muted">{{ change.before }} → {{ change.after }}</span>
                  </template>
                </el-table-column>
              </el-table>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="batch_no" label="批次号" width="150" />
        <el-table-column v-if="!repair" prop="repair_no" label="维修单号" width="140" />
        <el-table-column label="结果变化" width="150">
          <template #default="{ row }">
            <StatusTag :dict="REPAIR_RESULT" :value="row.old_result" />
            <el-icon class="arrow"><Right /></el-icon>
            <StatusTag :dict="REPAIR_RESULT" :value="row.new_result" />
          </template>
        </el-table-column>
        <el-table-column prop="operator" label="操作人" width="90" />
        <el-table-column label="生效月" width="90">
          <template #default="{ row }">
            {{ row.effect_month }}
            <el-tooltip v-if="row.source_settled" content="源月份已结算, 调整计入生效月" placement="top">
              <el-tag size="small" type="warning" effect="plain">跨月</el-tag>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column prop="reason" label="更正原因" min-width="150" show-overflow-tooltip />
        <el-table-column label="更正时间" width="150">
          <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!loading && rows.length === 0" description="暂无更正记录" :image-size="80" />

      <DataPagination
        v-if="!repair"
        :page="query.page"
        :page-size="query.page_size"
        :total="total"
        @page-change="(p) => { query.page = p; load() }"
        @size-change="(s) => { query.page_size = s; query.page = 1; load() }"
      />
    </div>
  </el-drawer>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { Right } from '@element-plus/icons-vue'
import StatusTag from '@/components/common/StatusTag.vue'
import DataPagination from '@/components/common/DataPagination.vue'
import { repairApi } from '@/api/repair'
import { REPAIR_LIABILITY, REPAIR_NATURE, REPAIR_RESULT } from '@/constants/dict'
import { formatDateTime } from '@/utils/format'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  // 传入维修记录时展示该记录的修订路径, 否则展示全局更正记录(分页)。
  repair: { type: Object, default: null },
})

defineEmits(['update:modelValue'])

const loading = ref(false)
const rows = ref([])
const total = ref(0)
const expanded = ref([])
const query = reactive({ page: 1, page_size: 10 })

const DICTS = { result: REPAIR_RESULT, liability: REPAIR_LIABILITY, nature: REPAIR_NATURE }
const dictOf = (field) => DICTS[field] ?? REPAIR_RESULT

async function load() {
  loading.value = true
  try {
    if (props.repair) {
      rows.value = await repairApi.correctionsByRepair(props.repair.id)
      total.value = rows.value.length
    } else {
      const data = await repairApi.corrections({ page: query.page, page_size: query.page_size })
      rows.value = data?.items ?? []
      total.value = data?.total ?? 0
    }
    // 默认展开第一条的逐字段对照, 便于直接查看
    expanded.value = rows.value.length ? [rows.value[0].id] : []
  } catch (error) {
    rows.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.drawer-body {
  padding: 0 4px;
}

.path-hint {
  margin-bottom: 12px;
}

.arrow {
  vertical-align: middle;
  margin: 0 2px;
}

.changes-panel {
  padding: 8px 16px;
  background: var(--el-fill-color-light);
}

.changes-title {
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 8px;
}
</style>
