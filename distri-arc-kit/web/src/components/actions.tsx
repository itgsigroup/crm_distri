import { Icon } from './Icon'
import { SheetHead, useFeedback } from './feedback'
import { PENDING_ORCH } from '../lib/i18n/id'

/**
 * Action button of a list row (mockup actBtn). Until proposals exist (stage 05/06) it opens a sheet that
 * explains no proposal is waiting yet; nothing is ever sent from here.
 */
export function ActBtn({ label, icon, small, dealer }: { label: string; icon: string; small?: boolean; dealer?: string }) {
  const { openSheet, closeSheet } = useFeedback()
  return (
    <button
      className="btn primary"
      style={small ? { height: 28, fontSize: 12 } : undefined}
      onClick={() =>
        openSheet(
          <>
            <SheetHead icon={icon} title={label} sub={dealer ? `${dealer} · tenggat —` : 'Beberapa dealer'} onClose={closeSheet} />
            <div className="sec"><h4><span className="ai" />Status</h4><p>{PENDING_ORCH}. Agen menyiapkan alasan, draft pesan, dan langkah setelah disetujui; Anda yang memutuskan.</p></div>
            <div className="ft">
              <button className="btn primary" disabled><Icon name="check" />Setujui &amp; jalankan</button>
              <button className="btn quiet" onClick={closeSheet}>Nanti</button>
              <span className="spacer" />
              <span className="pol"><Icon name="lock" />Tidak ada yang terkirim ke dealer tanpa langkah ini</span>
            </div>
          </>,
        )
      }
    >
      <Icon name={icon} />
      {label}
    </button>
  )
}
