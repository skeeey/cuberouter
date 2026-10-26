/*
Copyright (C) 2023-2026 QuantumNous
Copyright (C) 2026 CubeRouter

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
import { useTranslation } from 'react-i18next'

import {
  OrganizationSection,
  OrganizationSectionEmpty,
} from './organization-section'
import { OrganizationTaskList } from './organization-task-list'

type OrganizationTasksSectionProps = {
  organizationId: number
  /** The caller may read the organization's task records. */
  canView: boolean
  /**
   * The caller reads every member's tasks, not just their own.
   *
   * Without it the backend pins the list to the caller, so the holder column
   * would repeat one name down the table.
   */
  canViewWideData: boolean
  /** Leaves the page when the backend refuses the read. */
  onForbidden: () => void
}

/**
 * What the organization's keys submitted, and what it cost.
 *
 * Midjourney is temporarily disabled: the platform has no Midjourney upstream,
 * so the section deliberately has no tab strip while there is only one panel.
 * `OrganizationMidjourneyTaskList` and its endpoint remain for the day it is
 * re-enabled.
 */
export function OrganizationTasksSection(props: OrganizationTasksSectionProps) {
  const { t } = useTranslation()

  if (!props.canView) {
    return (
      <OrganizationSectionEmpty
        icon='tasks'
        title={t('Tasks')}
        message={t(
          'Only an organization owner or administrator can read its task records.'
        )}
      />
    )
  }

  return (
    <OrganizationSection
      icon='tasks'
      title={t('Tasks')}
      description={t(
        'What the organization submitted through its keys, and what each task cost.'
      )}
      scroll
    >
      <OrganizationTaskList
        organizationId={props.organizationId}
        canViewWideData={props.canViewWideData}
        onForbidden={props.onForbidden}
      />
    </OrganizationSection>
  )
}
