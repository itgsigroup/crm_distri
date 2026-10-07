import { useMe } from '../../app/queries'
import { RolesCard, UsersCard } from './UsersCard'

/** Sidebar → Pengguna: the user master (accounts, one WhatsApp number each) and the role master. */
export function UsersPage() {
  const { data: me } = useMe()
  if (me && !me.manage_users) return <div className="card"><p style={{ margin: 0, fontSize: 13, color: 'var(--text-2)' }}>Master pengguna untuk CEO dan admin.</p></div>
  return (
    <div className="ai-grid">
      <div className="stack"><UsersCard /></div>
      <div className="stack"><RolesCard /></div>
    </div>
  )
}
