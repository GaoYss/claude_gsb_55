<template>
  <div class="page">
    <PageHeader title="班组归集" description="按 班组 × 责任方 × 维修性质 归集月度维修量与费用, 统计口径为当前生效结果">
      <el-button :icon="Refresh" @click="load">刷新</el-button>
      <el-button :icon="Document" @click="correctionsVisible = true">更正记录</el-button>
      <el-button v-if="!data.settled" type="primary" :icon="Lock" @click="settleVisible = true">结算该月</el-button>
    </PageHeader>

    <el-card shadow="never">
      <div class="filter-bar">
        <el-date-picker
          v-model="month"
          type="month"
          value-format="YYYY-MM"
          :clearable="false"
          placeholder="选择月份"
          @change="load"
        />
        <el-tag v-if="data.settled" type="success" effect="dark">
          已结算 · {{ data.settled_by }} · {{ formatDateTime(data.settled_at) }}
        </el-tag>
        <el-tag v-else type="warning" effect="plain">未结算 · 实时归集, 结果更正后自动刷新</el-tag>
      </div>
      <el-alert v-if="data.settled" type="success" :closable="false" class="settled-hint">
        该月已结算, 归集金额为结算时封存的快照; 结算后的跨月更正不影响本页金额, 调整计入更正发生月。
      </el-alert>
    </el-card>

    <el-card shadow="never">
      <div class="section-title">归集明细</div>
      <el-table v-loading="loading" :data="data.rows" stripe show-summary :summary-method="summarize">
        <el-table-column label="维修班组" min-width="140">
          <template #default="{ row }">{{ row.repair_team || '未分班' }}</template>
        </el-table-column>
        <el-table-column label="责任方" width="130">
          <template #default="{ row }"><StatusTag :dict="REPAIR_LIABILITY" :value="row.liability" /></template>
        </el-table-column>
        <el-table-column label="维修性质" width="130">
          <template #default="{ row }"><StatusTag :dict="REPAIR_NATURE" :value="row.nature" /></template>
        </el-table-column>
        <el-table-column prop="repair_count" label="维修次数" width="110" align="right" />
        <el-table-column label="费用合计" width="140" align="right">
          <template #default="{ row }">{{ formatMoney(row.total_cost) }}</template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!loading && data.rows.length === 0" description="该月暂无完工维修记录" :image-size="80" />
    </el-card>

    <el-card shadow="never">
      <div class="section-title">
        <span>跨月更正调整</span>
        <el-tag size="small" type="info" effect="plain">源月份已结算, 金额影响计入本月</el-tag>
      </div>
      <el-table :data="data.adjustments" size="small" border>
        <el-table-column prop="repair_no" label="维修单号" width="150" />
        <el-table-column label="班组" min-width="120">
          <template #default="{ row }">{{ row.repair_team || '未分班' }}</template>
        </el-table-column>
        <el-table-column prop="source_month" label="源月份" width="90" />
        <el-table-column label="结果变化" width="160">
          <template #default="{ row }">
            <StatusTag :dict="REPAIR_RESULT" :value="row.old_result" />
            <el-icon class="arrow"><Right /></el-icon>
            <StatusTag :dict="REPAIR_RESULT" :value="row.new_result" />
          </template>
        </el-table-column>
        <el-table-column label="责任方变化" width="200">
          <template #default="{ row }">
            <StatusTag :dict="REPAIR_LIABILITY" :value="row.old_liability" />
            <el-icon class="arrow"><Right /></el-icon>
            <StatusTag :dict="REPAIR_LIABILITY" :value="row.new_liability" />
          </template>
        </el-table-column>
        <el-table-column label="调整金额" width="110" align="right">
          <template #default="{ row }">{{ formatMoney(row.cost) }}</template>
        </el-table-column>
        <el-table-column prop="operator" label="操作人" width="90" />
        <el-table-column label="更正时间" width="150">
          <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
        </el-table-column>
      </el-table>
      <el-empty v-if="data.adjustments.length === 0" description="本月无跨月更正调整" :image-size="80" />
    </el-card>

    <el-dialog v-model="settleVisible" title="结算确认" width="480px">
      <el-alert type="warning" :closable="false" class="settle-hint">
        结算后 {{ month }} 的班组归集金额将封存, 之后对该月记录的结果更正不再影响该月归集, 调整计入更正发生月。结算不可撤销。
      </el-alert>
      <el-form label-width="90px">
        <el-form-item label="结算月份">
          <el-input :model-value="month" disabled />
        </el-form-item>
        <el-form-item label="结算人" required>
          <el-input v-model="settleForm.settled_by" maxlength="64" placeholder="必填" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="settleForm.note" maxlength="255" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="settleVisible = false">取消</el-button>
        <el-button type="primary" :loading="settling" @click="handleSettle">确认结算</el-button>
      </template>
    </el-dialog>

    <CorrectionsDrawer v-model="correctionsVisible" />
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Document, Lock, Refresh, Right } from '@element-plus/icons-vue'
import PageHeader from '@/components/common/PageHeader.vue'
import StatusTag from '@/components/common/StatusTag.vue'
import CorrectionsDrawer from '@/views/repair/components/CorrectionsDrawer.vue'
import { repairApi } from '@/api/repair'
import { REPAIR_LIABILITY, REPAIR_NATURE, REPAIR_RESULT } from '@/constants/dict'
import { formatDateTime, formatMoney } from '@/utils/format'

const now = new Date()
const month = ref(`${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`)

const loading = ref(false)
const settling = ref(false)
const settleVisible = ref(false)
const correctionsVisible = ref(false)

const data = ref({ month: month.value, settled: false, rows: [], adjustments: [] })
const settleForm = reactive({ settled_by: '', note: '' })

async function load() {
  loading.value = true
  try {
    data.value = await repairApi.aggregation(month.value)
  } catch (error) {
    data.value = { month: month.value, settled: false, rows: [], adjustments: [] }
  } finally {
    loading.value = false
  }
}

async function handleSettle() {
  if (!settleForm.settled_by.trim()) {
    ElMessage.warning('请填写结算人')
    return
  }
  settling.value = true
  try {
    await repairApi.settle({ month: month.value, ...settleForm })
    ElMessage.success(`${month.value} 已结算, 归集金额已封存`)
    settleVisible.value = false
    load()
  } finally {
    settling.value = false
  }
}

function summarize({ columns, data: rows }) {
  return columns.map((column, index) => {
    if (index === 0) return '合计'
    if (column.property === 'repair_count') {
      return rows.reduce((sum, row) => sum + Number(row.repair_count ?? 0), 0)
    }
    if (index === 4) {
      const total = rows.reduce((sum, row) => sum + Number(row.total_cost ?? 0), 0)
      return formatMoney(total)
    }
    return ''
  })
}

onMounted(load)
</script>

<style scoped>
.filter-bar {
  display: flex;
  align-items: center;
  gap: 12px;
}

.settled-hint {
  margin-top: 12px;
}

.settle-hint {
  margin-bottom: 12px;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  margin-bottom: 12px;
}

.arrow {
  vertical-align: middle;
  margin: 0 2px;
}
</style>
