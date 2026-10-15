import { nothing, type PropertyDeclarations, type PropertyValues, type TemplateResult } from 'lit';
import { CivitaiElement } from './base.js';
/** The same three every other sized element takes, so a toolbar can match. */
export type FieldSize = 'sm' | 'md' | 'lg';
/** The `[data-civitai-ui-label|description|error|control]` chrome, scoped. */
export declare const fieldStyles: import("lit").CSSResult;
/**
 * A labelled control that participates in its form. Subclasses render the
 * control itself; everything around it — ids, `aria-describedby`, validity and
 * the value the form sees — is the same for all of them.
 */
export declare abstract class CivitaiField extends CivitaiElement {
    static formAssociated: boolean;
    static styles: import("lit").CSSResult[];
    static properties: PropertyDeclarations;
    name: string;
    /** Current value. The `value` ATTRIBUTE is the default, as on a native input. */
    value: string;
    label: string;
    description: string;
    /** Message to show; also marks the control invalid. */
    error: string;
    required: boolean;
    disabled: boolean;
    size: FieldSize;
    protected readonly internals: ElementInternals;
    protected readonly fieldId: string;
    constructor();
    get form(): HTMLFormElement | null;
    get validity(): ValidityState;
    checkValidity(): boolean;
    reportValidity(): boolean;
    /** Native semantics: reset returns to the `value` attribute, not to empty. */
    formResetCallback(): void;
    formStateRestoreCallback(state: string | null): void;
    protected get control(): HTMLElement | null;
    protected get invalid(): boolean;
    protected get describedBy(): string | undefined;
    connectedCallback(): void;
    protected updated(changed: PropertyValues): void;
    /** What the form submits. A control that can submit nothing answers `null`. */
    protected formValue(): string | File | FormData | null;
    /** Whether `required` is unsatisfied. */
    protected get missing(): boolean;
    protected get missingMessage(): string;
    protected sync(): void;
    protected onControlInput(event: Event): void;
    /**
     * A form does not cross a shadow boundary, so the inner control's implicit
     * submission never reaches it. Enter has to be forwarded by hand.
     */
    protected submitOnEnter(event: KeyboardEvent): void;
    protected abstract renderControl(): TemplateResult;
    protected renderRequiredMark(): TemplateResult | typeof nothing;
    protected renderLabel(): TemplateResult | typeof nothing;
    protected renderDescription(): TemplateResult | typeof nothing;
    protected renderError(): TemplateResult | typeof nothing;
    render(): TemplateResult;
    /** The attributes every control shares, so a subclass spells out only its own. */
    protected controlAttrs(): {
        id: string;
        part: string;
        name: string | undefined;
        ariaInvalid: string | undefined;
        ariaDescribedBy: string | undefined;
    };
}
