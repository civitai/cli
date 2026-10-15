import { type CSSResultGroup, type PropertyDeclarations, type PropertyValues, type TemplateResult } from 'lit';
import { CivitaiElement } from './base.js';
export type MediaStatus = 'loading' | 'loaded' | 'error' | 'blocked';
/**
 * What `<civitai-image>`, `<civitai-video>` and `<civitai-audio>` share: a file
 * that may still be being made (`pending`), may be withheld (`blocked`), and may
 * fail to load (`fallback`), with the outcome reflected as `status`.
 */
export declare abstract class CivitaiMediaElement extends CivitaiElement {
    #private;
    static styles: CSSResultGroup;
    static properties: PropertyDeclarations;
    src: string;
    alt: string;
    /** Text shown when the file fails to load; the `fallback` slot wins over it. */
    fallback: string;
    /** Still being made: a loader shows and nothing is requested. */
    pending: boolean;
    /** Withheld from this viewer: the `blocked` slot says why, and the file is never requested. */
    blocked: boolean;
    status: MediaStatus;
    constructor();
    /** The `<img>`, `<video>` or `<audio>`, reporting back through `settleLoaded` and `settleFailed`. */
    protected abstract renderMedia(): TemplateResult;
    protected settleLoaded: () => void;
    protected settleFailed: () => void;
    protected willUpdate(changed: PropertyValues): void;
    protected updated(changed: PropertyValues): void;
    render(): TemplateResult;
}
