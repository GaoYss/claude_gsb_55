<template>
  <div class="page">
    <PageHeader title="班组归集" description="按班组与维修结果归集月度工作量与金额, 结算后金额冻结, 跨月更正以调整项计入当前月">
      <el-date-picker
        v-model="month"
        type="month"
        value-format="YYYY-MM"
        :clearable="false"
        placeholder="选择月份"
        @change="load"
      />
      <el-button :icon="Refresh" @click="load">刷新</el-button>
      <el-button v-if="data && !data.settled" type="primary" :icon="Lock" :disabled="!settleable" @click="handleSettle">
        结算本月
      </el-button>
    </PageHeader>

    <div v-loading="loading">
      <el-alert v-if="data?.settled" type="success" :closable="false" class="status-bar">
        {{ data.month }} 已于 {{ formatDateTime(data.settled_at) }} 由 {{ data.settled_by }} 结算,
        归集金额已冻结; 之后的跨月更正不会改写本月数字, 以调整项计入其生效月。
      </el-alert>
      <el-alert v-else-if="data" type="info" :closable="false" class="status-bar">
        {{ data.month }} 尚未结算, 以下为按最新结果实时归集的数字, 结果更正后自动刷新。
      </el-alert>

      <el-empty v-if="data && !data.teams.length" description="本月没有已完工的维修记录" />

      <el-row v-else :gutter="16">
        <el-col v-for="team in data?.teams ?? []" :key="team.repair_team || '__none__'" :xs="24" :md="12" :lg="8">
          <el-card shadow="never" class="team-card">
            <template #header>
              <div class="team-head">
                <span class="team-name">{{ team.repair_team || '未分配班组' }}</span>
                <span class="team-total">{{ team.total_count }} 次 / {{ formatMoney(team.total_cost) }}</span>
              </div>
            </template>
            <el-table :data="team.by_result" size="small" border>
              <el-table-column label="结果" width="90">
                <template #default="{ row }">
                  <StatusTag :dict="REPAIR_RESULT" :value="row.result" />
                </template>
              </el-table-column>
              <el-table-column prop="liable_party" label="责任方" min-width="100" />
              <el-table-column prop="repair_nature" label="维修性质" min-width="100" />
              <el-table-column prop="count" label="次数" width="60" align="right" />
              <el-table-column label="金额" width="110" align="right">
                <template #default="{ row }">{{ formatMoney(row.cost) }}</template>
              </el-table-column>
            </el-table>
          </el-card>
        </el-col>
      </el-row>

      <el-card v-if="data?.adjustments?.length" shadow="never" class="adjustment-card">
        <template #header>
          <div class="adjustment-head">
            <span>跨月更正调整项</span>
            <span class="text-muted">原完工月已结算, 以下更正不计入历史月, 调整计入 {{ data.month }}</span>
          </div>
        </template>
        <el-table :data="data.adjustments" size="small" border stripe>
          <el-table-column prop="revision_no" label="修订单号" width="150" />
          <el-table-column prop="repair_no" label="维修单号" width="150" />
          <el-table-column label="班组" min-width="110">
            <template #default="{ row }">{{ row.repair_team || '未分配班组' }}</template>
          </el-table-column>
          <el-table-column prop="origin_month" label="原完工月" width="90" />
          <el-table-column label="结果更正" width="150">
            <template #default="{ row }">
              {{ dictLabel(REPAIR_RESULT, row.old_result) }} → {{ dictLabel(REPAIR_RESULT, row.new_result) }}
            </template>
          </el-table-column>
          <el-table-column label="金额调整" width="110" align="right">
            <template #default="{ row }">
              <span :class="row.cost_delta < 0 ? 'delta-negative' : 'delta-positive'">
                {{ row.cost_delta > 0 ? '+' : '' }}{{ formatMoney(row.cost_delta) }}
              </span>
            </template>
          </el-table-column>
          <el-table-column prop="reason" label="更正原因" min-width="160" show-overflow-tooltip />
          <el-table-column prop="operator" label="操作人" width="90" />
          <el-table-column prop="created_at" label="登记时间" width="150" />
        </el-table>
      </el-card>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Lock, Refresh } from '@element-plus/icons-vue'
import PageHeader from '@/components/common/PageHeader.vue'
import StatusTag from '@/components/common/StatusTag.vue'
import { repairApi } from '@/api/repair'
import { REPAIR_RESULT, dictLabel } from '@/constants/dict'
import { formatDateTime, formatMoney } from '@/utils/format'

const loading = ref(false)
const data = ref(null)

const now = new Date()
const currentMonth = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
const month = ref(currentMonth)

// 只允许结算已结束的月份。
const settleable = computed(() => month.value < currentMonth)

async function load() {
  loading.value = true
  try {
    data.value = await repairApi.aggregation({ month: month.value })
  } finally {
    loading.value = false
  }
}

async function handleSettle() {
  let operator = ''
  try {
    const result = await ElMessageBox.prompt(
      `结算后 ${month.value} 的班组归集金额将冻结, 之后的跨月更正只能以调整项计入后续月份。`,
      '结算确认',
      {
        confirmButtonText: '确认结算',
        cancelButtonText: '取消',
        inputPlaceholder: '请输入操作人',
        inputValidator: (value) => (value && value.trim() ? true : '操作人不能为空'),
      },
    )
    operator = result.value.trim()
  } catch (error) {
    return
  }

  try {
    await repairApi.settle({ month: month.value, operator })
    ElMessage.success(`${month.value} 已完成结算`)
    await load()
  } catch (error) {
    // 错误提示由请求拦截器统一处理
  }
}

onMounted(load)
</script>

<style scoped>
.status-bar {
  margin-bottom: 16px;
}

.team-card {
  margin-bottom: 16px;
}

.team-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.team-name {
  font-weight: 600;
}

.team-total {
  color: var(--el-color-primary);
  font-weight: 600;
}

.adjustment-card {
  margin-top: 8px;
}

.adjustment-head {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.delta-negative {
  color: var(--el-color-danger);
}

.delta-positive {
  color: var(--el-color-success);
}
</style>
