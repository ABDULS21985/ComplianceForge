'use client';

import { Loader2 } from 'lucide-react';
import type { ReactNode } from 'react';

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { Button, type ButtonProps } from '@/components/ui/button';
import { cn } from '@/lib/utils';

interface ConfirmActionProps {
  actionLabel: string;
  children: ReactNode;
  description: string;
  disabled?: boolean;
  onConfirm: () => void;
  pending?: boolean;
  title: string;
  triggerAriaLabel?: string;
  triggerSize?: ButtonProps['size'];
  triggerVariant?: ButtonProps['variant'];
}

export function ConfirmAction({
  actionLabel,
  children,
  description,
  disabled,
  onConfirm,
  pending,
  title,
  triggerAriaLabel,
  triggerSize = 'sm',
  triggerVariant = 'outline',
}: ConfirmActionProps) {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button
          type="button"
          size={triggerSize}
          variant={triggerVariant}
          disabled={disabled || pending}
          aria-label={triggerAriaLabel}
        >
          {pending && <Loader2 aria-hidden="true" className="mr-2 h-4 w-4 animate-spin" />}
          {children}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className={cn(triggerVariant === 'destructive' && 'bg-destructive text-destructive-foreground hover:bg-destructive/90')}
          >
            {actionLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
