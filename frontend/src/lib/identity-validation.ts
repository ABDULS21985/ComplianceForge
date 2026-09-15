import { z } from 'zod';

export const organizationIdentitySchema = z.object({
  organization_id: z.string().uuid('Enter the organization UUID from your administrator.'),
  email: z.string().trim().email('Enter a valid email address.'),
});

export const loginIdentitySchema = organizationIdentitySchema.extend({
  password: z.string().min(8, 'Password must be at least 8 characters.').max(72),
});

const newPassword = z
  .string()
  .min(12, 'Use at least 12 characters.')
  .max(72, 'Use no more than 72 characters.')
  .refine((value) => value.trim() === value, 'Password cannot begin or end with spaces.');

const passwordFields = { password: newPassword, confirm_password: z.string() };

export const newPasswordSchema = z
  .object(passwordFields)
  .refine((value) => value.password === value.confirm_password, {
    message: 'Passwords do not match.',
    path: ['confirm_password'],
  });

export const invitationAcceptanceSchema = z
  .object({
    ...passwordFields,
    first_name: z.string().trim().max(100, 'Use no more than 100 characters.'),
    last_name: z.string().trim().max(100, 'Use no more than 100 characters.'),
  })
  .refine((value) => value.password === value.confirm_password, {
    message: 'Passwords do not match.',
    path: ['confirm_password'],
  });

export type InvitationAcceptanceValues = z.infer<typeof invitationAcceptanceSchema>;
export type LoginIdentityValues = z.infer<typeof loginIdentitySchema>;
export type NewPasswordValues = z.infer<typeof newPasswordSchema>;
export type OrganizationIdentityValues = z.infer<typeof organizationIdentitySchema>;
