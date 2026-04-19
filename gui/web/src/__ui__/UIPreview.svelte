<script lang="ts">
  import { Button, Card, Input, Badge, Icon, StatRow, Section, Spinner, Empty, ErrorBanner } from '@/ui'
  import { Switch, Dialog, Select, Tabs, Tooltip, DropdownMenu, Combobox } from '@/ui'
  import { theme } from '@/lib/theme.svelte'

  let dialogOpen = $state(false)
  let switched = $state(false)
  let selectVal = $state<'h3' | 'reality' | 'cdn'>('h3')
  let tabsVal = $state<'a' | 'b' | 'c'>('a')
  let combo = $state<string | undefined>(undefined)
  let inputVal = $state('')
</script>

<div class="root">
  <header class="bar">
    <h2>UI Preview · P1</h2>
    <Button variant="ghost" size="sm" onclick={() => theme.toggle()}>
      Toggle theme ({theme.current})
    </Button>
  </header>

  <Section title="Button">
    <div class="row">
      <Button variant="primary">Primary</Button>
      <Button variant="secondary">Secondary</Button>
      <Button variant="ghost">Ghost</Button>
      <Button variant="danger">Danger</Button>
      <Button loading>Loading</Button>
      <Button disabled>Disabled</Button>
      <Button size="sm">Small</Button>
    </div>
  </Section>

  <Section title="Card + StatRow">
    <Card>
      <StatRow label="RTT" value="42 ms" mono />
      <StatRow label="Loss" value="0.0 %" />
      <StatRow label="Transport" value="H3 / BBR" />
    </Card>
  </Section>

  <Section title="Input">
    <Input label="Server name" placeholder="eg. sg-hk-02" bind:value={inputVal} />
    <Input label="With error" error="This field is required" />
  </Section>

  <Section title="Badge">
    <div class="row">
      <Badge>neutral</Badge>
      <Badge variant="success">success</Badge>
      <Badge variant="warning">warning</Badge>
      <Badge variant="danger">danger</Badge>
      <Badge variant="info">info</Badge>
    </div>
  </Section>

  <Section title="Icons (16 registered)">
    <div class="row">
      <Icon name="dashboard" size={18} />
      <Icon name="servers" size={18} />
      <Icon name="subscriptions" size={18} />
      <Icon name="groups" size={18} />
      <Icon name="routing" size={18} />
      <Icon name="mesh" size={18} />
      <Icon name="logs" size={18} />
      <Icon name="settings" size={18} />
      <Icon name="check" size={18} />
      <Icon name="x" size={18} />
      <Icon name="plus" size={18} />
      <Icon name="trash" size={18} />
      <Icon name="info" size={18} />
      <Icon name="chevronLeft" size={18} />
      <Icon name="chevronRight" size={18} />
      <Icon name="chevronDown" size={18} />
    </div>
  </Section>

  <Section title="State primitives">
    <div class="row"><Spinner size={20} /></div>
    <Empty icon="servers" title="No servers" description="Add one to get started" />
    <ErrorBanner message="Connection refused" onretry={() => alert('retry')} />
  </Section>

  <Section title="Switch">
    <Switch bind:checked={switched} label="Enable telemetry" />
    <p>Current: {switched}</p>
  </Section>

  <Section title="Select">
    <Select
      value={selectVal}
      options={[
        { value: 'h3', label: 'HTTP/3' },
        { value: 'reality', label: 'Reality' },
        { value: 'cdn', label: 'CDN' },
      ]}
      onValueChange={(v) => (selectVal = v)}
    />
    <p>Selected: {selectVal}</p>
  </Section>

  <Section title="Tabs">
    <Tabs
      value={tabsVal}
      items={[
        { value: 'a', label: 'Overview' },
        { value: 'b', label: 'Detail' },
        { value: 'c', label: 'History' },
      ]}
      onValueChange={(v) => (tabsVal = v)}
    />
    <p>Active: {tabsVal}</p>
  </Section>

  <Section title="Tooltip">
    <Tooltip content="I am a tooltip.">
      <Button variant="ghost">Hover me</Button>
    </Tooltip>
  </Section>

  <Section title="DropdownMenu">
    <DropdownMenu items={[
      { label: 'Rename', onselect: () => alert('rename') },
      { label: 'Duplicate', onselect: () => alert('dup') },
      { label: 'Delete', onselect: () => alert('del'), danger: true },
    ]}>
      <Button variant="secondary">Actions ▾</Button>
    </DropdownMenu>
  </Section>

  <Section title="Combobox">
    <Combobox
      value={combo}
      items={Array.from({ length: 20 }, (_, i) => ({ value: `opt-${i}`, label: `Option ${i}` }))}
      onValueChange={(v) => (combo = v)}
    />
    <p>Selected: {combo ?? '(none)'}</p>
  </Section>

  <Section title="Dialog">
    <Button variant="primary" onclick={() => (dialogOpen = true)}>Open dialog</Button>
    <Dialog bind:open={dialogOpen} title="Delete server?" description="This cannot be undone.">
      Are you sure you want to delete <strong>sg-hk-02</strong>?
      {#snippet actions()}
        <Button variant="ghost" onclick={() => (dialogOpen = false)}>Cancel</Button>
        <Button variant="danger" onclick={() => (dialogOpen = false)}>Delete</Button>
      {/snippet}
    </Dialog>
  </Section>
</div>

<style>
  .root {
    padding: var(--shuttle-space-5) var(--shuttle-space-7);
    max-width: 960px; margin: 0 auto;
    background: var(--shuttle-bg-base);
    color: var(--shuttle-fg-primary);
    font-family: var(--shuttle-font-sans);
    min-height: 100vh;
  }
  .bar {
    display: flex; align-items: center; justify-content: space-between;
    margin-bottom: var(--shuttle-space-5);
  }
  h2 {
    margin: 0;
    font-size: var(--shuttle-text-xl);
    letter-spacing: var(--shuttle-tracking-tight);
  }
  .row {
    display: flex; flex-wrap: wrap; align-items: center;
    gap: var(--shuttle-space-3);
  }
  p {
    margin-top: var(--shuttle-space-2);
    font-size: var(--shuttle-text-sm);
    color: var(--shuttle-fg-secondary);
  }
</style>
