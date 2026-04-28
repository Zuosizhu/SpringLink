<script>
  import { afterUpdate } from 'svelte'

  export let logs = []
  export let onClearLogs = () => {}

  let logPanel
  let autoScroll = true

  afterUpdate(() => {
    if (autoScroll && logPanel) {
      logPanel.scrollTop = logPanel.scrollHeight
    }
  })
</script>

<div class="log-section">
  <div class="log-header">
    <span class="section-title">日志输出区</span>
    <div class="log-header-actions">
      <button class="btn-ghost btn-sm" on:click={() => autoScroll = !autoScroll} title={autoScroll ? '停止自动滚动' : '开启自动滚动'}>
        {autoScroll ? '⏸ 暂停' : '▶ 滚动'}
      </button>
      <button class="btn-ghost btn-sm" on:click={onClearLogs}>清空</button>
    </div>
  </div>
  <div class="log-panel" bind:this={logPanel}>
    {#each logs as log}
      <div class="log-line">
        <span class="log-cmd">[{log.cmd}]</span>
        <span class="log-msg">{log.line}</span>
      </div>
    {:else}
      <div class="log-empty">暂无日志...</div>
    {/each}
  </div>
</div>

<style>
  .log-section {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 200px;
  }

  .log-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .section-title {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  .log-header-actions {
    display: flex;
    gap: 6px;
  }

  .log-panel {
    flex: 1;
    background: var(--bg-log);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 8px 12px;
    overflow-y: auto;
    font-family: 'Cascadia Code', 'Fira Code', 'Consolas', monospace;
    font-size: 12px;
    line-height: 1.6;
    user-select: text;
    -webkit-user-select: text;
  }

  .log-line {
    color: var(--text-secondary);
    word-break: break-all;
  }

  .log-cmd {
    color: var(--accent);
    margin-right: 8px;
  }

  .log-empty {
    color: var(--text-muted);
    text-align: center;
    padding: 20px;
  }
</style>
