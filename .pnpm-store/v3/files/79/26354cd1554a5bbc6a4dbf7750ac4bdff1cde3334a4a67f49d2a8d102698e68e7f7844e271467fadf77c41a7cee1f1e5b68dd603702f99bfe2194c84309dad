import { ErrorObject } from 'ajv';

declare var betterAjvErrors: betterAjvErrors.IBetterAjvErrors;

export = betterAjvErrors;
export as namespace betterAjvErrors;

declare namespace betterAjvErrors {
  export interface IInputOptions {
    format?: 'cli' | 'js';
    indent?: number | null;
  }

  export interface IOutputError {
    start: { line: number; column: number; offset: number };
    end?: { line: number; column: number; offset: number };
    error: string;
    suggestion?: string;
  }

  export interface IBetterAjvErrors {
    (
      schema: any,
      data: any,
      errors?: ErrorObject[] | null,
      options?: betterAjvErrors.IInputOptions
    ): betterAjvErrors.IOutputError[] | void;
  }
}
