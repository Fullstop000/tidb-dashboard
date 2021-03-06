import { Alert, Col, Row } from 'antd';
import React, { useState} from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from "react-router-dom";
import { SearchHeader, SearchProgress, SearchResult } from './components';

export default function LogSearchingDetail() {
  const { t } = useTranslation()
  const { id } = useParams()
  const taskGroupID = id === undefined ? 0 : +id

  const [tasks, setTasks] = useState([])

  return (
    <div>
      <Row gutter={[16, 16]}>
        <Col span={18}>
          <SearchHeader taskGroupID={taskGroupID} />
          <Alert
            message={t('log_searching.page.tip')}
            type="info"
            showIcon
            style={{ marginTop: 14, marginBottom: 14 }}
          />
          <SearchResult taskGroupID={taskGroupID} tasks={tasks} />
        </Col>
        <Col span={6}>
          <SearchProgress taskGroupID={taskGroupID} tasks={tasks} setTasks={setTasks} />
        </Col>
      </Row>
    </div>
  )
}
