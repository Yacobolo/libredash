import { css } from 'lit'

export const visualActionStyles = css`
  .visual-actions {
    display: flex;
    flex: 0 0 auto;
    align-items: center;
    gap: var(--base-size-4);
  }

  .icon-action {
    display: grid;
    width: var(--control-xsmall-size);
    height: var(--control-xsmall-size);
    min-height: var(--control-xsmall-size);
    place-items: center;
    border: var(--ld-border-transparent);
    border-radius: var(--ld-radius-tight);
    background: transparent;
    color: var(--ld-icon-muted, var(--ld-fg-muted));
    cursor: pointer;
    padding: 0;
    font: inherit;
    line-height: var(--ld-line-height-none);
  }

  .icon-action svg {
    width: var(--base-size-16);
    height: var(--base-size-16);
  }

  .icon-action:hover,
  .icon-action:focus-visible {
    border-color: var(--ld-line-default);
    background: var(--ld-bg-panel-muted);
    color: var(--ld-icon-default, var(--ld-fg-default));
    outline: 0;
  }
`
