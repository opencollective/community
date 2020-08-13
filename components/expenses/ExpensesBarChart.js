import React from 'react';
import {withApollo} from '@apollo/client/react/hoc';
import {Bar} from 'react-chartjs-2';
import {injectIntl} from "react-intl";
import PropTypes from "prop-types";
import moment from 'moment';
import {orderBy} from 'lodash';

class ExpensesBarChart extends React.Component {

  static propTypes = {
    expenses: PropTypes.object,
  };

  constructor(props) {
    super(props);
    this.state = {data: {
        labels: [],
        datasets: []
      }, width: 800, height: 300};
  }

  componentDidMount() {
    this.formatData(this.props.expenses);
  }

  /**
   * Format data in chart.js format
   *
   * @param source
   */
  formatData(source) {

    const periods = [];
    const cats = [];

    source = orderBy(source.nodes, (item) => item.createdAt)

    source.map(item => {

      const month = new moment(item.createdAt).format('MM/YYYY');

      let monthKey = periods.findIndex((i) => i === month);

      if (monthKey === -1) {
        monthKey = periods.push(month);
      }

      let catKey = cats.findIndex((i) => i.label === item.tags[0]);

      if (catKey === -1) {
        catKey = cats.push({
          label: item.tags[0] ?? 'undefined',
          data: [],
          backgroundColor: null,
          borderColor: null,
          borderWidth: 1,
          stack: 'default'
        });

        catKey = catKey - 1;

        cats[catKey]['backgroundColor'] = this.generateRainbow(12, catKey + 1);
        cats[catKey]['borderColor'] = this.generateRainbow(12, catKey + 1);
      }

      /**
       * Apply a different color based on index + number of tags
       */
      if (typeof cats[catKey] !== 'undefined') {

        if (typeof cats[catKey]['data'][monthKey] === 'undefined') {
          cats[catKey]['data'][monthKey] = 0;
        }

        cats[catKey]['data'][monthKey] += item.amount / 100;
      }
    });

    const data = {
      labels: periods,
      datasets: cats
    }

    console.log('datasets', cats);

    this.setState({data: data});
  }

  /**
   * Generate a nuance based color somewhere over the rainbow ♫
   *
   * @param numOfSteps
   * @param step
   * @returns {string}
   */
  generateRainbow(numOfSteps, step) {
    let r, g, b;
    const h = step / numOfSteps;
    const i = ~~(h * 6);
    const f = h * 6 - i;
    const q = 1 - f;
    switch (i % 6) {
      case 0:
        r = 1;
        g = f;
        b = 0;
        break;
      case 1:
        r = q;
        g = 1;
        b = 0;
        break;
      case 2:
        r = 0;
        g = 1;
        b = f;
        break;
      case 3:
        r = 0;
        g = q;
        b = 1;
        break;
      case 4:
        r = f;
        g = 0;
        b = 1;
        break;
      case 5:
        r = 1;
        g = 0;
        b = q;
        break;
    }
    const c = `#${(`00${(~~(r * 255)).toString(16)}`).slice(-2)}${(`00${(~~(g * 255)).toString(16)}`).slice(-2)}${(`00${(~~(b * 255)).toString(16)}`).slice(-2)}`;
    return (c);
  }

  render() {
    return (
      <div>
        <Bar
          data={this.state.data}
          width={this.state.width}
          height={this.state.height}
          options={{
            maintainAspectRatio: true
          }}
        />
      </div>
    );
  }
}

export default injectIntl(withApollo(ExpensesBarChart));
