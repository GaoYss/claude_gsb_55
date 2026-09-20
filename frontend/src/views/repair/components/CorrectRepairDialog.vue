<template>
  <el-dialog
    :model-value="modelValue"
    title="更正维修结果"
    width="640px"
    @update:model-value="$emit('update:modelValue', $event)"
    @open="syncForm"
  >
    <el-alert type="warning" :closable="false" class="correct-tip">
      更正不会改写原维修记录的结果与完工时间, 而是生成一条可追溯的修订记录;
      统计与班组归集按更正后的最新结果计算。若原完工月已结算, 本次更正将以调整项计入当前开放月。
    </el-alert>

    <el-descriptions v-if="model" :column="2" border size="small" class="repair-summary">
      <el-descriptions-item label="维修单号">{{ model.repair_no }}</el-descriptions-item>
      <el-descriptions-item label="完工时间">{{ formatDateTime(model.finished_at) }}</el-descriptions-item>
      <el-descriptions-item label="当前生效结果">
        <StatusTag :dict="REPAIR_RESULT" :value="model.current_result || model.result" />
      </el-descriptions-item>
      <el-descriptions-item label="当前生效金额">{{ formatMoney(model.current_cost ?? model.cost) }}</el-descriptions-item>
      <el-descriptions-item label="原始登记结果">
        <StatusTag :dict="REPAIR_RESULT" :value="model.result" />
        <span v-if="model.revision_count > 0" class="text-muted">(已更正 {{ model.revision_count }} 次)</span>
      </el-descriptions-item>
      <el-descriptions-item label="原始登记金额">{{ formatMoney(model.cost) }}</el-descriptions-item>
    </el-descriptions>

    <el-form ref="formRef" :model="form" :rules="rules" label-width="110px">
      <el-form-item label="更正后结果" prop="result">
        <el-select v-model="form.result" placeholder="保持不变" clearable style="width: 100%">
          <el-option v-for="(item, key) in REPAIR_RESULT" :key="key" :label="item.label" :value="key" />
        </el-select>
        <div v-if="form.result" class="form-hint text-muted">
          责任方: {{ resultMeta(form.result, 'liable_party') }} / 维修性质: {{ resultMeta(form.result, 'repair_nature') }}
        </div>
      </el-form-item>
      <el-form-item label="更正后金额" prop="cost">
        <el-input-number v-model="form.cost" :min="0" :precision="2" :step="10" placeholder="保持不变" style="width: 100%" />
        <div class="form-hint text-muted">结果与金额至少更正一项, 不修改的项留空即可。</div>
      </el-form-item>
      <el-form-item label="更正原因" prop="reason">
        <el-input v-model="form.reason" type="textarea" :rows="2" maxlength="255" show-word-limit placeholder="必填, 将写入修订记录供追溯" />
      </el-form-item>
      <el-form-item label="操作人" prop="operator">
        <el-input v-model="form.operator" maxlength="64" placeholder="必填" />
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="$emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">提交更正</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import StatusTag from '@/components/common/StatusTag.vue'
import { repairApi } from '@/api/repair'
import { REPAIR_RESULT, resultMeta } from '@/constants/dict'
import { formatDateTime, formatMoney } from '@/utils/format'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  model: { type: Object, default: null },
})

const emit = defineEmits(['update:modelValue', 'saved'])

const formRef = ref(null)
const submitting = ref(false)

const createForm = () => ({
  result: '',
  cost: null,
  reason: '',
  operator: '',
})

const form = reactive(createForm())

const rules = {
  reason: [{ required: true, message: '请填写更正原因', trigger: 'blur' }],
  operator: [{ required: true, message: '请填写操作人', trigger: 'blur' }],
}

function syncForm() {
  Object.assign(form, createForm())
}

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  if (!form.result && (form.cost === null || form.cost === undefined)) {
    ElMessage.warning('结果与金额至少更正一项')
    return
  }

  submitting.value = true
  try {
    const payload = { reason: form.reason.trim(), operator: form.operator.trim() }
    if (form.result) payload.result = form.result
    if (form.cost !== null && form.cost !== undefined) payload.cost = form.cost
    const result = await repairApi.correct(props.model.id, payload)
    emit('update:modelValue', false)
    emit('saved')

    const revision = result?.revisions?.[0]
    if (revision?.cross_settled) {
      ElMessageBox.alert(
        `原完工月 ${revision.origin_month} 已完成结算, 历史月归集金额不变; 本次更正已作为调整项计入 ${revision.effective_month}。`,
        '跨月更正已登记',
        { confirmButtonText: '知道了', type: 'info' },
      )
    } else {
      ElMessage.success('更正已生效')
    }
  } catch (error) {
    // 错误提示由请求拦截器统一处理
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.correct-tip {
  margin-bottom: 16px;
}

.repair-summary {
  margin-bottom: 16px;
}

.form-hint {
  font-size: 12px;
  line-height: 1.6;
}
</style>
