import { FC } from "react"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { Replica } from "api/typesGenerated"

export interface ReplicasTableProps {
    replicas: Replica[]
}

export const ReplicasTable: FC<ReplicasTableProps> = ({ replicas }) => {
  return (
    <TableContainer>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell width="33%">Hostname</TableCell>
            <TableCell width="33%">Info</TableCell>
            <TableCell width="33%">State</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {replicas.map((replica) => (
            <TableRow key={replica.id}>
              <TableCell>
                {replica.hostname}
              </TableCell>
              <TableCell>
                Database Latency: {replica.database_latency / 1000}ms Relay
                Address: {replica.relay_address}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  )
}
