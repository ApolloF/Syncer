<script lang="ts">
  import { slide } from 'svelte/transition'
  import Icon from '../lib/Icon.svelte'
  import Modal from '../lib/Modal.svelte'
  import { ui, attempt, fail, refresh, toast } from '../lib/state.svelte'
  import { bytes } from '../lib/fmt'
  import { idSegments, selfName } from '../lib/devices'
  import { Devices, AddDevice, DismissDevice, RemoveDevice, AcceptDevice, AllowJoin, IgnoreJoin, CopyText, PasteText } from '../../wailsjs/go/app/App'
  import type { app } from '../../wailsjs/go/models'

  // How a linked PC is reached (DeviceView.via).
  const viaText: Record<string, string> = { lan: 'on this network', direct: 'over the internet', relay: 'through a Syncthing relay (encrypted)' }

  let v = $state<app.DevicesView | null>(null)
  let id = $state('')
  let name = $state('')
  let linking = $state(false)
  let removeFor = $state<app.DeviceView | null>(null)
  let showQR = $state(false)
  // A PC to check the ID of before it is linked (a request) or allowed (a PC that offered to join).
  let confirmFor = $state<{ id: string; name: string; join: boolean } | null>(null)

  async function load() {
    try { v = await Devices() } catch (e) { if (ui.overview?.syncthing.running) fail(e) }
  }
  $effect(() => { ui.tick; load() })

  async function link(devId = id, devName = name) {
    linking = true
    if (await attempt(() => AddDevice(devId, devName), 'Linked. Saves will start syncing in a moment.')) {
      id = ''; name = ''
      load(); refresh()
    }
    linking = false
  }

  async function confirmed() {
    const c = confirmFor!
    confirmFor = null
    linking = true
    const ok = c.join
      ? await attempt(() => AllowJoin(c.id), `${c.name} can sync your saves now`)
      : await attempt(() => AcceptDevice(c.id), 'Linked. Saves will start syncing in a moment.')
    if (ok) { load(); refresh() }
    linking = false
  }

  async function ignore(d: app.JoinView) {
    await attempt(async () => IgnoreJoin(d.id))
    load(); refresh()
  }

  async function paste() {
    const t = (await PasteText())?.trim()
    if (t) id = t
  }

  function copy() {
    if (!v) return
    CopyText(v.myID)
    toast('ID copied', 'ok')
  }
</script>

<header>
  <h1>Devices</h1>
  <p class="muted">Link your PCs once. From then on every game save follows you.</p>
</header>

{#if !ui.overview?.syncthing.running}
  <div class="card empty"><Icon name="alert" size={22} /><p>Sync isn't running. Start it from the Overview.</p></div>
{:else}
  {#each v?.pending ?? [] as p (p.id)}
    <div class="card request" transition:slide={{ duration: 160 }}>
      <div class="ic"><Icon name="devices" /></div>
      <div class="grow">
        <h2>{selfName(p.name)} wants to link</h2>
        <p class="mono faint ellipsis">{p.id}</p>
      </div>
      <button class="btn ghost" onclick={async () => { await attempt(() => DismissDevice(p.id)); load(); refresh() }}>Ignore</button>
      <button class="btn primary" disabled={linking} onclick={() => (confirmFor = { id: p.id, name: selfName(p.name), join: false })}>Review</button>
    </div>
  {/each}

  {#each v?.requests ?? [] as d (d.id)}
    <div class="card request" transition:slide={{ duration: 160 }}>
      <div class="ic"><Icon name="devices" /></div>
      <div class="grow">
        <h2>{d.name} wants to join as one of your PCs</h2>
        <p class="muted small">It is already connected to this PC and says it runs Syncer. Allow it only if it is your own PC running Syncer: it gets all your game saves.</p>
      </div>
      <button class="btn ghost" onclick={() => ignore(d)}>Ignore</button>
      <button class="btn primary" onclick={() => (confirmFor = { id: d.id, name: d.name, join: true })}>Allow</button>
    </div>
  {/each}

  <section class="grid">
    <div class="card me">
      <div class="row">
        <h2 class="grow">This PC</h2>
        <span class="muted">{v?.myName}</span>
      </div>
      <p class="muted small">Enter this ID on your other PC.</p>
      <button class="idbox selectable" onclick={copy} title="Copy">
        {#each idSegments(v?.myID ?? '') as s}<span class="grp">{s.group}</span>{#if s.dash}<span class="sep">-</span><wbr />{/if}{/each}
      </button>
      <div class="row">
        <button class="btn" onclick={copy}><Icon name="copy" size={15} /> Copy ID</button>
        <button class="btn ghost" onclick={() => (showQR = true)}>Show QR</button>
      </div>
    </div>

    <div class="card">
      <h2>Link another PC</h2>
      <ol class="muted small">
        <li>Install and open Syncer on the other PC.</li>
        <li>Paste its ID below — or paste this PC's ID over there.</li>
        <li>Choose Review on the other PC and check the ID matches. Done.</li>
      </ol>
      <p class="faint small">If Windows asks whether Syncthing may use the network, choose Allow, or your other PCs can't reach this one.</p>
      <div class="field">
        <input type="text" class="mono" placeholder="XXXXXXX-XXXXXXX-…" bind:value={id} spellcheck="false" />
        <button class="btn icon" title="Paste" onclick={paste}><Icon name="paste" size={16} /></button>
      </div>
      <div class="row">
        <input type="text" class="grow" placeholder="Name (optional), e.g. Laptop" bind:value={name} />
        <button class="btn primary" disabled={!id.trim() || linking} onclick={() => link()}>
          {#if linking}<Icon name="refresh" size={15} class="spin" />{:else}<Icon name="link" size={15} />{/if}
          Link
        </button>
      </div>
    </div>
  </section>

  {#if v?.ignored?.length}
    <h2 class="section">Ignored</h2>
    <div class="card flush list">
      {#each v.ignored as d (d.id)}
        <div class="item">
          <div class="grow">
            <div class="name">{d.name}</div>
            <div class="faint small">Asked to join as one of your PCs. Syncer leaves it alone.</div>
          </div>
          <button class="btn" onclick={() => (confirmFor = { id: d.id, name: d.name, join: true })}>Allow</button>
        </div>
      {/each}
    </div>
  {/if}

  {#if v?.devices?.length}
    <h2 class="section">Linked PCs</h2>
    <div class="card flush list">
      {#each v.devices as d (d.id)}
        <div class="item" transition:slide={{ duration: 150 }}>
          <span class="dot {d.connected ? 'ok' : ''}"></span>
          <div class="grow">
            <div class="name">{d.name || d.id.slice(0, 7)}</div>
            <div class="faint small ellipsis">
              {#if d.connected}Online · {viaText[d.via] ?? 'connected'}{d.address ? ` · ${d.address.replace(/^[a-z]+:\/\//, '')}` : ''}{:else}Offline — syncs when both PCs are on; newer saves it backed up still come from its backup{/if}
            </div>
          </div>
          {#if d.versionGap === 'older'}<span class="pill warn" title="This PC runs Syncer {ui.overview?.version}. Open Syncer on {d.name || 'that PC'} and update it under Settings, or leave it in the tray and it updates by itself.">Needs update{d.version ? ` · ${d.version}` : ''}</span>
          {:else if d.versionGap === 'newer'}<span class="pill warn" title="That PC runs a newer Syncer than this one ({ui.overview?.version}). Update this PC under Settings.">Newer Syncer · {d.version}</span>{/if}
          {#if d.connected}
            {#if d.completion >= 100}<span class="pill ok">In sync</span>
            {:else}<span class="pill accent">{Math.floor(d.completion)}% · {bytes(d.needBytes)} left</span>{/if}
          {/if}
          <button class="btn ghost icon sm danger" title="Unlink" onclick={() => (removeFor = d)}><Icon name="trash" size={16} /></button>
        </div>
      {/each}
    </div>
  {/if}
{/if}

{#if showQR && v}
  <Modal title="This PC's ID" onclose={() => (showQR = false)}>
    <img class="qr" src={v.qr} alt="QR code of device ID" />
    <p class="idbox center selectable">
      {#each idSegments(v.myID) as s}<span class="grp">{s.group}</span>{#if s.dash}<span class="sep">-</span><wbr />{/if}{/each}
    </p>
    {#snippet actions()}<button class="btn" onclick={() => (showQR = false)}>Close</button>{/snippet}
  </Modal>
{/if}

{#if confirmFor}
  <Modal title={confirmFor.join ? 'Allow this PC?' : 'Link this PC?'} onclose={() => (confirmFor = null)}>
    <p>
      {confirmFor.join ? 'A PC that is already connected to this one says it runs Syncer. If you allow it, it gets all your game saves.' : 'A PC asks to link with yours and get all your game saves.'}
      The name it gives itself is <strong>{confirmFor.name}</strong>, but anyone can pick any name.
    </p>
    <p>Open Devices on your other PC and check that its ID is exactly this one:</p>
    <p class="idbox selectable">
      {#each idSegments(confirmFor.id) as s}<span class="grp">{s.group}</span>{#if s.dash}<span class="sep">-</span><wbr />{/if}{/each}
    </p>
    <p>If it differs, or you don't know this PC, choose Cancel.</p>
    {#snippet actions()}
      <button class="btn" onclick={() => (confirmFor = null)}>Cancel</button>
      <button class="btn primary" onclick={confirmed}>{confirmFor?.join ? 'The ID matches: allow' : 'The ID matches: link'}</button>
    {/snippet}
  </Modal>
{/if}

{#if removeFor}
  <Modal title="Unlink {removeFor.name || 'this PC'}?" onclose={() => (removeFor = null)}>
    <p>Saves stop syncing with that PC. Nothing is deleted on either side.</p>
    {#snippet actions()}
      <button class="btn" onclick={() => (removeFor = null)}>Cancel</button>
      <button class="btn primary" onclick={async () => { const d = removeFor!; removeFor = null; await attempt(() => RemoveDevice(d.id), 'Unlinked'); load(); refresh() }}>Unlink</button>
    {/snippet}
  </Modal>
{/if}

<style>
  header { display: flex; flex-direction: column; gap: 4px; }
  .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
  .grid .card { display: flex; flex-direction: column; gap: 12px; }
  .small { font-size: 13px; }
  .idbox {
    font-family: var(--mono); font-size: 13px; line-height: 1.7; text-align: left;
    padding: 12px 14px; border-radius: 8px; border: 1px dashed var(--border);
    background: var(--surface-2); color: var(--text); cursor: pointer;
    transition: border-color .15s;
  }
  .idbox:hover { border-color: var(--accent); }
  .grp { white-space: nowrap; }
  .sep { color: var(--faint); }
  ol { margin: 0; padding-left: 18px; display: flex; flex-direction: column; gap: 3px; }
  .field { display: flex; gap: 8px; }
  .field input { flex: 1; min-width: 0; }
  .request { display: flex; align-items: center; gap: 14px; border-color: var(--accent); }
  .request .mono { font-size: 11.5px; max-width: 440px; }
  .ic {
    width: 38px; height: 38px; border-radius: 10px; flex: none; display: grid; place-items: center;
    background: var(--accent-soft); color: var(--accent);
  }
  .section { margin-top: 6px; }
  .name { font-weight: 500; }
  .empty { display: flex; flex-direction: column; align-items: center; gap: 12px; padding: 40px; color: var(--muted); }
  .qr { width: 220px; height: 220px; align-self: center; border-radius: 8px; background: #fff; padding: 8px; image-rendering: pixelated; }
  .center { text-align: center; font-size: 11.5px; word-break: break-all; }
  @media (max-width: 860px) { .grid { grid-template-columns: 1fr; } }
</style>
