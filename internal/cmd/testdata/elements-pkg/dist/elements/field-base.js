import { css, html, nothing } from 'lit';
import { CivitaiElement } from './base.js';
import { hostBaseline } from './shared-styles.js';
let sequence = 0;
/** The `[data-civitai-ui-label|description|error|control]` chrome, scoped. */
export const fieldStyles = css `
  :host {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  label {
    font-size: 14px;
    font-weight: 600;
    color: var(--civitai-color-text);
  }
  .required {
    color: var(--civitai-color-error);
    margin-left: 2px;
  }
  .description {
    font-size: 12px;
    color: var(--civitai-color-text-dimmed);
  }
  .error {
    font-size: 12px;
    color: var(--civitai-color-error);
  }
  .control {
    width: 100%;
    font-family: var(--civitai-font);
    font-size: 14px;
    color: var(--civitai-color-text);
    background: var(--civitai-color-surface);
    border: 1px solid var(--civitai-color-border);
    border-radius: var(--civitai-radius);
    padding: 8px 12px;
    transition: border-color 120ms ease;
  }
  .control:focus {
    outline: none;
    border-color: var(--civitai-color-primary);
  }
  .control[aria-invalid='true'] {
    border-color: var(--civitai-color-error);
  }
  .control:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  :host([size='sm']) .control {
    padding: 4px 8px;
    font-size: 13px;
  }
  :host([size='lg']) .control {
    padding: 11px 14px;
    font-size: 15px;
  }
  :host([size='sm']) label {
    font-size: 13px;
  }
  :host([size='lg']) label {
    font-size: 15px;
  }
`;
/**
 * A labelled control that participates in its form. Subclasses render the
 * control itself; everything around it — ids, `aria-describedby`, validity and
 * the value the form sees — is the same for all of them.
 */
export class CivitaiField extends CivitaiElement {
    static formAssociated = true;
    static styles = [hostBaseline, fieldStyles];
    static properties = {
        name: { reflect: true },
        value: {},
        label: { reflect: true },
        description: { reflect: true },
        error: { reflect: true },
        required: { type: Boolean, reflect: true },
        disabled: { type: Boolean, reflect: true },
        size: { reflect: true },
    };
    internals = this.attachInternals();
    fieldId = `ci-field-${(sequence += 1)}`;
    constructor() {
        super();
        this.name = '';
        this.value = '';
        this.label = '';
        this.description = '';
        this.error = '';
        this.required = false;
        this.disabled = false;
        this.size = 'md';
    }
    get form() {
        return this.internals.form;
    }
    get validity() {
        return this.internals.validity;
    }
    checkValidity() {
        return this.internals.checkValidity();
    }
    reportValidity() {
        return this.internals.reportValidity();
    }
    /** Native semantics: reset returns to the `value` attribute, not to empty. */
    formResetCallback() {
        this.value = this.getAttribute('value') ?? '';
    }
    formStateRestoreCallback(state) {
        this.value = state ?? '';
    }
    /* The PART, not a class: a checkbox marks its input with one but has no use
       for the `.control` box styling, and both still need a validity anchor. */
    get control() {
        return this.renderRoot.querySelector('[part~="control"]');
    }
    get invalid() {
        return this.error !== '';
    }
    get describedBy() {
        return ([
            this.description !== '' ? `${this.fieldId}-desc` : '',
            this.invalid ? `${this.fieldId}-err` : '',
        ]
            .filter(Boolean)
            .join(' ') || undefined);
    }
    connectedCallback() {
        super.connectedCallback();
        this.sync();
    }
    updated(changed) {
        super.updated(changed);
        this.sync();
    }
    /** What the form submits. A control that can submit nothing answers `null`. */
    formValue() {
        return this.value;
    }
    /** Whether `required` is unsatisfied. */
    get missing() {
        return this.value === '';
    }
    get missingMessage() {
        return 'Please fill out this field.';
    }
    sync() {
        this.internals.setFormValue(this.formValue());
        const anchor = this.control ?? undefined;
        if (this.required && this.missing) {
            this.internals.setValidity({ valueMissing: true }, this.error || this.missingMessage, anchor);
        }
        else if (this.invalid) {
            this.internals.setValidity({ customError: true }, this.error, anchor);
        }
        else {
            this.internals.setValidity({});
        }
    }
    onControlInput(event) {
        this.value = event.target.value;
    }
    /**
     * A form does not cross a shadow boundary, so the inner control's implicit
     * submission never reaches it. Enter has to be forwarded by hand.
     */
    submitOnEnter(event) {
        if (event.key !== 'Enter' || event.defaultPrevented)
            return;
        const { form } = this.internals;
        if (!form)
            return;
        event.preventDefault();
        form.requestSubmit();
    }
    renderRequiredMark() {
        return this.required ? html `<span class="required" aria-hidden="true">*</span>` : nothing;
    }
    renderLabel() {
        if (this.label === '')
            return nothing;
        return html `<label for=${this.fieldId} part="label"
      >${this.label}${this.renderRequiredMark()}</label
    >`;
    }
    renderDescription() {
        if (this.description === '')
            return nothing;
        return html `<span id="${this.fieldId}-desc" class="description" part="description"
      >${this.description}</span
    >`;
    }
    renderError() {
        if (!this.invalid)
            return nothing;
        return html `<span id="${this.fieldId}-err" class="error" part="error" role="alert"
      >${this.error}</span
    >`;
    }
    render() {
        return html `
      ${this.renderLabel()}${this.renderDescription()}${this.renderControl()}${this.renderError()}
    `;
    }
    /** The attributes every control shares, so a subclass spells out only its own. */
    controlAttrs() {
        return {
            id: this.fieldId,
            part: 'control',
            name: this.name || undefined,
            ariaInvalid: this.invalid ? 'true' : undefined,
            ariaDescribedBy: this.describedBy,
        };
    }
}
