// SplitTypeTag 分摊方式标签测试：列表与详情共用的标签组件，
// 四种分摊方式都必须显示对应中文文案（业务规则 R-Tag）。
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import SplitTypeTag from '@/components/SplitTypeTag.vue'

const CASES = [
  { type: 'equal', label: '均摊' },
  { type: 'ratio', label: '按比例' },
  { type: 'amount', label: '按金额' },
  { type: 'share', label: '按份额' },
] as const

describe('SplitTypeTag 分摊方式标签', () => {
  for (const { type, label } of CASES) {
    it(`split_type=${type} 显示「${label}」`, () => {
      const wrapper = mount(SplitTypeTag, {
        props: { type },
        global: { plugins: [ElementPlus] },
      })
      expect(
        wrapper.text(),
        `违反业务规则[分摊方式标签应显示对应中文文案]: split_type=${type} 应显示「${label}」，实际渲染为「${wrapper.text() || '(空白)'}」`,
      ).toContain(label)
      // 标签不得渲染为空白（防止组件未注册导致的空渲染回归）
      const tag = wrapper.find('.el-tag')
      expect(tag.exists(), `违反业务规则[分摊方式标签必须实际渲染]: split_type=${type} 未渲染出 el-tag 元素`).toBe(true)
    })
  }
})
