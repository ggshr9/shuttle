import { describe, it, expect } from 'vitest'
import { matchesFilter, subServerAddrs } from './filter'
import type { Server, Subscription, GroupInfo } from '@/lib/api/types'

const manual: Server = { addr: '1.1.1.1:443', name: 'manual' }
const fromSub1: Server = { addr: '2.2.2.2:443', name: 'sub1-a' }
const fromSub2: Server = { addr: '3.3.3.3:443', name: 'sub1-b' }
const inGroup: Server = { addr: '4.4.4.4:443', name: 'in-group' }

const subs: Subscription[] = [
  { id: 'sub1', name: 'Sub One', url: 'https://one', servers: [fromSub1, fromSub2] },
  { id: 'sub2', name: 'Sub Two', url: 'https://two', servers: [] },
]

const groups: GroupInfo[] = [
  { tag: 'premium', strategy: 'rr', members: ['4.4.4.4:443', '1.1.1.1:443'] },
  { tag: 'empty',   strategy: 'rr', members: [] },
]

describe('subServerAddrs', () => {
  it('returns addrs of servers owned by any subscription', () => {
    const addrs = subServerAddrs(subs)
    expect(addrs.has('2.2.2.2:443')).toBe(true)
    expect(addrs.has('3.3.3.3:443')).toBe(true)
    expect(addrs.has('1.1.1.1:443')).toBe(false) // manual
  })
  it('empty when no subscriptions', () => {
    expect(subServerAddrs([]).size).toBe(0)
  })
})

describe('matchesFilter', () => {
  it("'all' matches every server", () => {
    for (const s of [manual, fromSub1, fromSub2, inGroup]) {
      expect(matchesFilter(s, 'all', subs, groups)).toBe(true)
    }
  })

  it("'manual' matches only servers outside every subscription", () => {
    expect(matchesFilter(manual, 'manual', subs, groups)).toBe(true)
    expect(matchesFilter(inGroup, 'manual', subs, groups)).toBe(true) // groups != subs
    expect(matchesFilter(fromSub1, 'manual', subs, groups)).toBe(false)
    expect(matchesFilter(fromSub2, 'manual', subs, groups)).toBe(false)
  })

  it("'subscriptions' matches any server owned by at least one sub", () => {
    expect(matchesFilter(fromSub1, 'subscriptions', subs, groups)).toBe(true)
    expect(matchesFilter(fromSub2, 'subscriptions', subs, groups)).toBe(true)
    expect(matchesFilter(manual, 'subscriptions', subs, groups)).toBe(false)
  })

  it("'subscription:<id>' matches only servers in that specific sub", () => {
    expect(matchesFilter(fromSub1, 'subscription:sub1', subs, groups)).toBe(true)
    expect(matchesFilter(fromSub1, 'subscription:sub2', subs, groups)).toBe(false)
    expect(matchesFilter(manual, 'subscription:sub1', subs, groups)).toBe(false)
  })

  it("'subscription:<unknown>' matches nothing (not everything)", () => {
    expect(matchesFilter(fromSub1, 'subscription:does-not-exist', subs, groups)).toBe(false)
  })

  it("'group:<tag>' matches only servers in that group's members", () => {
    expect(matchesFilter(inGroup, 'group:premium', subs, groups)).toBe(true)
    expect(matchesFilter(manual, 'group:premium', subs, groups)).toBe(true) // 1.1.1.1 also in premium
    expect(matchesFilter(fromSub1, 'group:premium', subs, groups)).toBe(false)
  })

  it("'group:<unknown>' matches nothing", () => {
    expect(matchesFilter(inGroup, 'group:ghost', subs, groups)).toBe(false)
  })

  it("'group:empty' matches nothing (valid group, empty members)", () => {
    expect(matchesFilter(inGroup, 'group:empty', subs, groups)).toBe(false)
  })

  it('unknown filter shape falls through to include-all', () => {
    // Defensive: a stale URL param shouldn't hide the whole server list.
    expect(matchesFilter(manual, 'garbage', subs, groups)).toBe(true)
  })
})
