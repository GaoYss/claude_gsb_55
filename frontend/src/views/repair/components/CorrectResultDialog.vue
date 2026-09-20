<template>
  <el-dialog
    :model-value="modelValue"
    title="维修结果更正"
    width="640px"
    @update:model-value="$emit('update:modelValue', $event)"
    @open="syncForm"
  >
    <template v-if="model">
      <el-alert type="info" :closable="false" class="correct-hint">
        更正不会改写原始结果与完工时间, 而是追加一条可追溯的修订记录, 统计与班组归集按最新结果重新计算;
        若完工月份已结算, 该月归集金额不变, 调整计入本月。
      </el-alert>

      <el-descriptions :column="2" border size="small" class="repair-summary">
        <el-descriptions-item label="维修单号">{{ model.repair_no }}</el-descriptions-item>
        <el-descriptions-item label="故障单号">{{ model.fault_no }}</el-descriptions-item>
        <el-descriptions-item label="维修班组">{{ model.repair_team || '-' }}</el-descriptions-item>
        <el-descriptions-item label="完工时间">{{ formatDateTime(model.finished_at) }}</el-descriptions-item>
        <el-descriptions-item label="当前结果">
          <RepairResultTag :row="model" />
        </el-descriptions-item>
        <el-descriptions-item label="原始结果">{{ dictLabel(REPAIR_RESULT, model.result) }}</el-descriptions-item>
      </el-descriptions>

      <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
        <el-form-item label="更正为" prop="new_result">
          <el-select v-model="form.new_result" style="width: 100%">
            <el-option
              v-for="(item, key) in REPAIR_RESULT"
              :key="key"
              :label="item.label"
              :value="key"
              :disabled="key === currentResult"
            />
          </el-select>
        </el-form-item>

        <el-form-item label="归属变化">
          <div class="attribution-preview">
            <div class="attribution-row">
              <span class="attribution-label">责任方</span>
              <StatusTag :dict="REPAIR_LIABILITY" :value="currentAttribution.liability" />
              <el-icon><Right /></el-icon>
              <StatusTag :dict="REPAIR_LIABILITY" :value="nextAttribution.liability" />
            </div>
            <div class="attribution-row">
              <span class="attribution-label">维修性质</span>
              <StatusTag :dict="REPAIR_NATURE" :value="currentAttribution.nature" />
              <el-icon><Right /></el-icon>
              <StatusTag :dict="REPAIR_NATURE" :value="nextAttribution.nature" />
            </div>
          </div>
        </el-form-item>

        <el-form-item label="更正原因" prop="reason">
          <el-input
            v-model="form.reason"
            type="textarea"
            :rows="2"
            maxlength="255"
            show-word-limit
            placeholder="必填, 将随修订记录一并留存"
          />
        </el-form-item>
        <el-form-item label="操作人" prop="operator">
          <el-input v-model="form.operator" maxlength="64" placeholder="必填, 更正经办人" />
        </el-form-item>
      </el-form>
    </template>

    <template #footer>
      <el-button @click="$emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">提交更正</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Right } from '@element-plus/icons-vue'
import StatusTag from '@/components/common/StatusTag.vue'
import RepairResultTag from '@/components/common/RepairResultTag.vue'
import { repairApi } from '@/api/repair'
import { REPAIR_LIABILITY, REPAIR_NATURE, REPAIR_RESULT, RESULT_ATTRIBUTION, dictLabel } from '@/constants/dict'
import { formatDateTime } from '@/utils/format'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  model: { type: Object, default: null },
})

const emit = defineEmits(['update:modelValue', 'saved'])

const formRef = ref(null)
const submitting = ref(false)

const form = reactive({ new_result: '', reason: '', operator: '' })

const rules = {
  new_result: [{ required: true, message: '请选择更正后的结果', trigger: 'change' }],
  reason: [{ required: true, message: '请填写更正原因', trigger: 'blur' }],
  operator: [{ required: true, message: '请填写操作人', trigger: 'blur' }],
}

const currentResult = computed(() => props.model?.current_result || props.model?.result || '')
const currentAttribution = computed(() => RESULT_ATTRIBUTION[currentResult.value] ?? {})
const nextAttribution = computed(() => RESULT_ATTRIBUTION[form.new_result] ?? {})

function syncForm() {
  form.new_result = ''
  form.reason = ''
}

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    await repairApi.correct(props.model.id, {
      operator: form.operator,
      new_result: form.new_result,
      reason: form.reason,
    })
    ElMessage.success('结果已更正, 统计与归集已同步刷新')
    emit('update:modelValue', false)
    emit('saved')
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.correct-hint {
  margin-bottom: 12px;
}

.repair-summary {
  margin-bottom: 16px;
}

.attribution-preview {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.attribution-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.attribution-label {
  width: 60px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
</style>
