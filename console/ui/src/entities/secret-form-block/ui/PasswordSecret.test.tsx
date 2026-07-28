import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom/vitest';
import { FormProvider, useForm } from 'react-hook-form';
import { describe, expect, it } from 'vitest';
import '@shared/i18n/i18n';
import PasswordSecretBlock from './PasswordSecret';

const PasswordSecretHarness = () => {
  const form = useForm({ defaultValues: { USERNAME: '', PASSWORD: '' } });
  return (
    <FormProvider {...form}>
      <PasswordSecretBlock />
    </FormProvider>
  );
};

describe('password secret form', () => {
  it('masks password input', () => {
    render(<PasswordSecretHarness />);
    expect(screen.getByLabelText(/Password/)).toHaveAttribute('type', 'password');
  });
});
