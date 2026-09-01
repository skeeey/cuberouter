/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Loader2 } from 'lucide-react'
import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatTimestamp } from '@/lib/format'
import { getRoleLabel } from '@/lib/roles'

import { getUserInvitees } from '../../api'
import { USER_STATUS } from '../../constants'
import type { InviteeBrief } from '../../types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number
  username: string
}

const PAGE_SIZE = 10

export function UserInviteesDialog({
  open,
  onOpenChange,
  userId,
  username,
}: Props) {
  const { t } = useTranslation()
  const [items, setItems] = useState<InviteeBrief[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const fetchPage = useCallback(
    async (targetPage: number) => {
      setLoading(true)
      setError('')
      try {
        const res = await getUserInvitees(userId, {
          p: targetPage,
          page_size: PAGE_SIZE,
        })
        if (res.success && res.data) {
          setItems(res.data.items)
          setTotal(res.data.total)
        } else {
          setError(res.message || t('Failed to load'))
        }
      } catch {
        setError(t('Failed to load'))
      } finally {
        setLoading(false)
      }
    },
    [userId, t]
  )

  useEffect(() => {
    if (open) {
      setPage(1)
      fetchPage(1)
    } else {
      setItems([])
      setTotal(0)
      setPage(1)
      setError('')
    }
  }, [open, fetchPage])

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  let body: ReactNode
  if (loading) {
    body = (
      <div className='flex items-center justify-center py-8'>
        <Loader2 className='text-muted-foreground h-6 w-6 animate-spin' />
      </div>
    )
  } else if (error) {
    body = <p className='text-destructive py-4 text-center text-sm'>{error}</p>
  } else {
    body = (
      <>
        <ScrollArea className='max-h-[50vh]'>
          {items.length === 0 ? (
            <p className='text-muted-foreground py-4 text-center text-sm'>
              {t('No invitees yet')}
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Username')}</TableHead>
                  <TableHead>{t('Email')}</TableHead>
                  <TableHead>{t('Phone')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Group')}</TableHead>
                  <TableHead>{t('Role')}</TableHead>
                  <TableHead>{t('Created At')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((invitee) => (
                  <TableRow key={invitee.id}>
                    <TableCell>{invitee.username}</TableCell>
                    <TableCell>{invitee.email || '-'}</TableCell>
                    <TableCell>{invitee.phone || '-'}</TableCell>
                    <TableCell>
                      {invitee.status === USER_STATUS.ENABLED
                        ? t('Enabled')
                        : t('Disabled')}
                    </TableCell>
                    <TableCell>{invitee.group}</TableCell>
                    <TableCell>{t(getRoleLabel(invitee.role))}</TableCell>
                    <TableCell>
                      {invitee.created_at
                        ? formatTimestamp(invitee.created_at)
                        : '-'}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </ScrollArea>

        <div className='flex items-center justify-end gap-2'>
          <Button
            variant='outline'
            size='sm'
            disabled={page <= 1}
            onClick={() => {
              const next = page - 1
              setPage(next)
              fetchPage(next)
            }}
          >
            {t('Previous')}
          </Button>
          <span className='text-muted-foreground text-xs'>
            {page} / {totalPages}
          </span>
          <Button
            variant='outline'
            size='sm'
            disabled={page >= totalPages}
            onClick={() => {
              const next = page + 1
              setPage(next)
              fetchPage(next)
            }}
          >
            {t('Next')}
          </Button>
        </div>
      </>
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Invitees')}
      description={`${username} (ID: ${userId})`}
      contentClassName='sm:max-w-2xl'
      contentHeight='auto'
      bodyClassName='space-y-4'
    >
      {body}
    </Dialog>
  )
}
