/**
 * BuiltInAuthFormValues describes a form using built-in (email/password)
 * authentication. This form may not always be present depending on external
 * auth providers available and administrative configurations
 */
export interface BuiltInAuthFormValues {
  email: string;
  password: string;
}
