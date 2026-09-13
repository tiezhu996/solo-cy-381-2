// ExpenseList 列表与详情的分摊方式展示测试：
// 四种分摊方式在列表「分摊方式」列显示对应文案；
// 按份额消费的详情显示「按份额」标签、每人份额与应付金额。
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ElementPlus from 'element-plus'

// 确定性测试数据（hoisted：供模块 mock 使用，无随机/时间依赖）
const mocks = vi.hoisted(() => {
  const share = (userId: number, username: string, nickname: string, shareAmount: number, ratio: number, shareCount: number) => ({
    user_id: userId,
    username,
    nickname,
    share_amount: shareAmount,
    ratio,
    share: shareCount,
    status: 'unsettled',
  })
  const expense = (id: number, title: string, amount: number, splitType: string, shares: unknown[]) => ({
    id,
    group_id: 1,
    title,
    amount,
    category: 'dining',
    payer_id: 1,
    payer_name: 'Alice',
    split_type: splitType,
    paid_at: '2026-08-01 12:00:00',
    receipt_url: '',
    status: 'active',
    created_by: 1,
    created_at: '2026-08-01 12:00:00',
    shares,
  })
  const expenses = [
    expense(1, '午餐', 90, 'equal', [share(1, 'alice', 'Alice', 30, 0.33, 0), share(2, 'bob', 'Bob', 30, 0.33, 0), share(3, 'carol', 'Carol', 30, 0.34, 0)]),
    expense(2, '打车', 60, 'ratio', [share(1, 'alice', 'Alice', 30, 0.5, 0), share(2, 'bob', 'Bob', 30, 0.5, 0)]),
    expense(3, '门票', 70, 'amount', [share(1, 'alice', 'Alice', 40, 0.57, 0), share(2, 'bob', 'Bob', 30, 0.43, 0)]),
    expense(4, '民宿', 100, 'share', [share(1, 'alice', 'Alice', 50, 0.5, 2), share(2, 'bob', 'Bob', 25, 0.25, 1), share(3, 'carol', 'Carol', 25, 0.25, 1)]),
  ]
  const members = [
    { id: 11, group_id: 1, user_id: 1, username: 'alice', nickname: 'Alice', avatar: '', role: 'owner', invited_by: 0, joined_at: '2026-08-01 00:00:00' },
    { id: 12, group_id: 1, user_id: 2, username: 'bob', nickname: 'Bob', avatar: '', role: 'normal', invited_by: 1, joined_at: '2026-08-01 00:00:00' },
    { id: 13, group_id: 1, user_id: 3, username: 'carol', nickname: 'Carol', avatar: '', role: 'normal', invited_by: 1, joined_at: '2026-08-01 00:00:00' },
  ]
  const group = { id: 1, name: '测试群组', description: '', owner_id: 1, status: 'active', member_count: 3, created_at: '2026-08-01 00:00:00' }
  return { expenses, members, group }
})

vi.mock('@/api/expense', () => ({
  listExpensesApi: vi.fn(() => Promise.resolve({ list: mocks.expenses, total: mocks.expenses.length, page: 1, page_size: 50 })),
  createExpenseApi: vi.fn(),
  getExpenseApi: vi.fn(),
  updateExpenseApi: vi.fn(),
  refundExpenseApi: vi.fn(),
  exportExpensesApi: vi.fn(() => '/groups/1/expenses/export?'),
}))

vi.mock('@/api/group', () => ({
  listMyGroupsApi: vi.fn(() => Promise.resolve({ list: [mocks.group], total: 1, page: 1, page_size: 50 })),
  getGroupApi: vi.fn(() => Promise.resolve(mocks.group)),
  createGroupApi: vi.fn(),
  updateGroupApi: vi.fn(),
  archiveGroupApi: vi.fn(),
  listMembersApi: vi.fn(() => Promise.resolve({ list: mocks.members, total: mocks.members.length })),
  inviteMemberApi: vi.fn(),
  removeMemberApi: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: '1' }, query: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

import ExpenseList from '@/pages/expense/ExpenseList.vue'

const SPLIT_LABELS = [
  { type: 'equal', label: '均摊' },
  { type: 'ratio', label: '按比例' },
  { type: 'amount', label: '按金额' },
  { type: 'share', label: '按份额' },
] as const

describe('ExpenseList 分摊方式展示', () => {
  let wrapper: VueWrapper | null = null

  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = null
    document.body.innerHTML = ''
  })

  async function mountPage() {
    wrapper = mount(ExpenseList, {
      global: { plugins: [ElementPlus] },
      attachTo: document.body,
    })
    await flushPromises()
    return wrapper
  }

  it('列表「分摊方式」列显示均摊/按比例/按金额/按份额四种文案', async () => {
    const w = await mountPage()
    for (const { type, label } of SPLIT_LABELS) {
      expect(
        w.text().includes(label),
        `违反业务规则[列表分摊方式列应显示对应文案]: split_type=${type} 的行应显示「${label}」`,
      ).toBe(true)
    }
    // 每种文案各出现一次（四条记录四种方式）
    for (const { label } of SPLIT_LABELS) {
      const tags = w.findAll('.el-tag').filter((t) => t.text() === label)
      expect(tags.length, `违反业务规则[列表分摊方式列应显示对应文案]: 「${label}」标签应出现 1 次，实际 ${tags.length} 次`).toBe(1)
    }
  })

  it('按份额消费的详情显示「按份额」标签、每人份额与应付金额', async () => {
    const w = await mountPage()
    const shareRow = w.findAll('tr').find((r) => r.text().includes('民宿'))
    expect(shareRow, '测试数据应包含「民宿」按份额消费行').toBeTruthy()
    const detailBtn = shareRow!.findAll('button').find((b) => b.text() === '详情')
    expect(detailBtn, '按份额消费行应有「详情」按钮').toBeTruthy()
    await detailBtn!.trigger('click')
    await flushPromises()

    const bodyText = document.body.textContent || ''
    expect(bodyText.includes('按份额'), '违反业务规则[详情分摊方式应显示「按份额」标签]').toBe(true)
    expect(bodyText.includes('份额'), '违反业务规则[按份额详情应展示每人份额]: 分摊明细应包含「份额」列').toBe(true)
    // 每人份额值与应付金额（50 = 100×2/4，25 = 100×1/4）
    for (const cell of ['2', '1', '50.00', '25.00']) {
      expect(bodyText.includes(cell), `违反业务规则[按份额详情应展示每人份额与应付金额]: 详情中应出现「${cell}」`).toBe(true)
    }
  })

  it('列表展开行显示按份额消费每人的份额与应付金额', async () => {
    const w = await mountPage()
    const shareRow = w.findAll('tr').find((r) => r.text().includes('民宿'))
    expect(shareRow, '测试数据应包含「民宿」按份额消费行').toBeTruthy()
    const expandIcon = shareRow!.find('.el-table__expand-icon')
    expect(expandIcon.exists(), '按份额消费行应有展开按钮').toBe(true)
    await expandIcon.trigger('click')
    await flushPromises()

    const text = w.text()
    for (const cell of ['份额 2', '份额 1', '50.00', '25.00']) {
      expect(text.includes(cell), `违反业务规则[列表展开行应展示每人份额与应付金额]: 展开内容应包含「${cell}」`).toBe(true)
    }
  })
})
