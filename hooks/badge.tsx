import { PALETTE } from './palette'

type Ui = Record<'Box' | 'Text' | 'Button', (props: any) => any>

export const makeBadge = ({ Box, Text, Button }: Ui) => {
  const Badge = ({ children, background, color = PALETTE.onBadge, bold }: { children: string; background: string; color?: string; bold?: boolean }) => (
    <Box alignSelf="flex-start" backgroundColor={background} paddingX={1}>
      <Text bold={bold} color={color}>{children}</Text>
    </Box>
  )

  const ActionButton = ({ id, label, background, onPress }: { id: string; label: string; background: string; onPress: () => void }) => (
    <Box alignSelf="flex-start" backgroundColor={background} paddingX={1}>
      <Button key={id} label={label} plain onPress={onPress}><Text color={PALETTE.onBadge}>{label}</Text></Button>
    </Box>
  )

  return { Badge, ActionButton }
}
