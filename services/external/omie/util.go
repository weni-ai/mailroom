package omie

import (
	"strconv"

	"github.com/nyaruka/goflow/assets"
	"github.com/pkg/errors"
)

func ParamsToIncluirContatoRequest(params []assets.ExternalServiceParam) (*IncluirContatoRequest, error) {
	r := &IncluirContatoRequest{}

	p := IncluirContatoRequestParam{}
	for _, param := range params {
		dv := param.Data.Value
		switch param.Type {
		case "identificacao":
			if param.Filter.Value != nil {
				switch param.Filter.Value.Name {
				case "nCod":
					v, err := strconv.Atoi(dv)
					if err != nil {
						return nil, errors.Wrap(err, "error to parse filter data value")
					}
					p.Identificacao.NCod = v
				case "cCodInt":
					p.Identificacao.CCodInt = dv
				case "cNome":
					p.Identificacao.CNome = dv
				case "cSobrenome":
					p.Identificacao.CSobrenome = dv
				case "cCargo":
					p.Identificacao.CCargo = dv
				case "dDtNasc":
					p.Identificacao.DDtNasc = dv
				case "nCodVend":
					v, err := strconv.Atoi(dv)
					if err != nil {
						return nil, errors.Wrap(err, "error to parse filter data value")
					}
					p.Identificacao.NCodVend = v
				case "nCodConta":
					v, err := strconv.Atoi(dv)
					if err != nil {
						return nil, errors.Wrap(err, "error to parse filter data value")
					}
					p.Identificacao.NCodConta = v
				}
			}
		case "endereco":
			if param.Filter.Value != nil {
				switch param.Filter.Value.Name {
				case "cEndereco":
					p.Endereco.CEndereco = dv
				case "cCompl":
					p.Endereco.CCompl = dv
				case "cCEP":
					p.Endereco.CCEP = dv
				case "cBairro":
					p.Endereco.CBairro = dv
				case "cCidade":
					p.Endereco.CCidade = dv
				case "cUF":
					p.Endereco.CUF = dv
				case "cPais":
					p.Endereco.CPais = dv
				}
			}
		case "telefone_email":
			if param.Filter.Value != nil {
				switch param.Filter.Value.Name {
				case "cDDDCel1":
					p.TelefoneEmail.CDDDCel1 = dv
				case "cNumCel1":
					p.TelefoneEmail.CNumCel1 = dv
				case "cDDDCel2":
					p.TelefoneEmail.CDDDCel2 = dv
				case "cNumCel2":
					p.TelefoneEmail.CNumCel2 = dv
				case "cDDDTel":
					p.TelefoneEmail.CDDDTel = dv
				case "cNumTel":
					p.TelefoneEmail.CNumTel = dv
				case "cDDDFax":
					p.TelefoneEmail.CDDDFax = dv
				case "cNumFax":
					p.TelefoneEmail.CNumFax = dv
				case "cEmail":
					p.TelefoneEmail.CEmail = dv
				case "cWebsite":
					p.TelefoneEmail.CWebsite = dv
				}
			}
		case "cObs":
			if param.Filter.Value != nil {
				p.CObs = dv
			}
		}
	}
	r.Param = append(r.Param, p)
	return r, nil
}

func ParamsToIncluirOportunidadeRequest(params []assets.ExternalServiceParam) (*IncluirOportunidadeRequest, error) {
	r := &IncluirOportunidadeRequest{}

	p := IncluirOportunidadeRequestParam{}
	for _, param := range params {
		dv := param.Data.Value
		switch param.Type {
		case "identificacao":
			switch param.Filter.Value.Name {
			case "cCodIntOp":
				p.Identificacao.CCodIntOp = dv
			case "cDesOp":
				p.Identificacao.CDesOp = dv
			case "nCodConta":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.Identificacao.NCodConta = v
			case "nCodContato":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.Identificacao.NCodContato = v
			case "nCodOp":
				v, err := strconv.ParseInt(dv, 10, 64)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.Identificacao.NCodOp = v
			case "nCodOrigem":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.Identificacao.NCodOrigem = v
			case "nCodSolucao":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.Identificacao.NCodSolucao = v
			case "nCodVendedor":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.Identificacao.NCodVendedor = v
			}
		case "fasesStatus":
			switch param.Filter.Value.Name {
			case "dConclusao":
				p.FasesStatus.DConclusao = dv
			case "dNovoLead":
				p.FasesStatus.DNovoLead = dv
			case "dProjeto":
				p.FasesStatus.DProjeto = dv
			case "dQualificacao":
				p.FasesStatus.DQualificacao = dv
			case "dShowRoom":
				p.FasesStatus.DShowRoom = dv
			case "dTreinamento":
				p.FasesStatus.DTreinamento = dv
			case "nCodFase":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.FasesStatus.NCodFase = v
			case "nCodMotivo":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.FasesStatus.NCodMotivo = v
			case "nCodStatus":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.FasesStatus.NCodStatus = v
			}
		case "ticket":
			switch param.Filter.Value.Name {
			case "nMeses":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.Ticket.NMeses = v
			case "nProdutos":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.Ticket.NProdutos = v
			case "nRecorrencia":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.Ticket.NRecorrencia = v
			case "nServicos":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.Ticket.NServicos = v
			case "nTicket":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.Ticket.NTicket = v
			}

		case "previsaoTemp":
			switch param.Filter.Value.Name {
			case "nAnoPrev":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.PrevisaoTemp.NAnoPrev = v
			case "nMesPrev":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.PrevisaoTemp.NMesPrev = v
			case "nTemperatura":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.PrevisaoTemp.NTemperatura = v
			}
		case "observacoes":
			p.Observacoes.CObs = dv
		case "outrasInf":
			switch param.Filter.Value.Name {
			case "cEmailOp":
				p.OutraInf.CEmailOp = dv
			case "dAlteracao":
				p.OutraInf.DAlteracao = dv
			case "dInclusao":
				p.OutraInf.DInclusao = dv
			case "hAlteracao":
				p.OutraInf.HAlteracao = dv
			case "hInclusao":
				p.OutraInf.HInclusao = dv
			case "nCodTipo":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.OutraInf.NCodTipo = v
			}
		case "envolvidos":
			switch param.Filter.Value.Name {
			case "nCodFinder":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.Envolvidos.NCodFinder = v
			case "nCodParceiro":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.Envolvidos.NCodParceiro = v
			case "nCodPrevenda":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.Envolvidos.NCodPrevenda = v
			}
		}
	}
	r.Param = append(r.Param, p)
	return r, nil
}

func ParamsToListarClientesRequest(params []assets.ExternalServiceParam) (*ListarClientesRequest, error) {
	r := &ListarClientesRequest{}

	p := ListarClientesRequestParam{}

	for _, param := range params {
		dv := param.Data.Value
		switch param.Type {
		case "pagina":
			v, err := strconv.Atoi(dv)
			if err != nil {
				return nil, errors.Wrap(err, "error to parse filter data value")
			}
			p.Pagina = v
		case "registros_por_pagina":
			v, err := strconv.Atoi(dv)
			if err != nil {
				return nil, errors.Wrap(err, "error to parse filter data value")
			}
			p.RegistrosPorPagina = v
		case "apenas_importado_api":
			p.ApenasImportadoAPI = dv
		case "ordenar_por":
			p.OrdenarPor = dv
		case "ordem_decrescente":
			p.OrdemDecrescente = dv
		case "filtrar_por_data_de":
			p.FiltrarPorDataDe = dv
		case "filtrar_por_data_ate":
			p.FiltrarPorDataAte = dv
		case "filtrar_por_hora_de":
			p.FiltrarPorHoraDe = dv
		case "filtrar_por_hora_ate":
			p.FiltrarPorHoraAte = dv
		case "filtrar_apenas_inclusao":
			p.FiltrarApenasInclusao = dv
		case "filtrar_apenas_alteracao":
			p.FiltrarApenasAlteracao = dv
		case "clientesFiltro":
			switch param.Filter.Value.Name {
			case "codigo_cliente_omie":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.ClientesFiltro.CodigoClienteOmie = v
			case "codigo_cliente_integracao":
				p.ClientesFiltro.CodigoClienteIntegracao = dv
			case "cnpj_cpf":
				p.ClientesFiltro.CnpjCpf = dv
			case "razao_social":
				p.ClientesFiltro.RazaoSocial = dv
			case "nome_fantasia":
				p.ClientesFiltro.NomeFantasia = dv
			case "endereco":
				p.ClientesFiltro.Endereco = dv
			case "bairro":
				p.ClientesFiltro.Bairro = dv
			case "cidade":
				p.ClientesFiltro.Cidade = dv
			case "estado":
				p.ClientesFiltro.Estado = dv
			case "cep":
				p.ClientesFiltro.Cep = dv
			case "contato":
				p.ClientesFiltro.Contato = dv
			case "email":
				p.ClientesFiltro.Email = dv
			case "homepage":
				p.ClientesFiltro.Homepage = dv
			case "inscricao_municipal":
				p.ClientesFiltro.InscricaoMunicipal = dv
			case "inscricao_estadual":
				p.ClientesFiltro.InscricaoEstadual = dv
			case "inscricao_suframa":
				p.ClientesFiltro.InscricaoSuframa = dv
			case "pessoa_fisica":
				p.ClientesFiltro.PessoaFisica = dv
			case "optante_simples_nacional":
				p.ClientesFiltro.OptanteSimplesNacional = dv
			case "inativo":
				p.ClientesFiltro.Inativo = dv
			case "tags":
				p.ClientesFiltro.Tags = dv
			}
		case "clientesPorCodigo":
			switch param.Filter.Value.Name {
			case "codigo_cliente_omie":
				v, err := strconv.Atoi(dv)
				if err != nil {
					return nil, errors.Wrap(err, "error to parse filter data value")
				}
				p.ClientesPorCodigo.CodigoClienteOmie = v
			case "codigo_cliente_integracao":
				p.ClientesPorCodigo.CodigoClienteIntegracao = dv
			}
		case "exibir_caracteristicas":
			p.ExibirCaracteristicas = dv
		}
	}
	r.Param = append(r.Param, p)
	return nil, nil
}

func ParamsToPesquisarLancamentosRequest(param []assets.ExternalServiceParam) (*PesquisarLancamentosRequest, error) {
	r := &PesquisarLancamentosRequest{}
	p := PesquisarLancamentosParam{}
	for _, param := range param {
		dv := param.Data.Value
		switch param.Type {
		case "nPagina":
			v, err := strconv.Atoi(dv)
			if err != nil {
				return nil, errors.Wrap(err, "error to parse filter data value")
			}
			p.NPagina = v
		case "nRegPorPagina":
			v, err := strconv.Atoi(dv)
			if err != nil {
				return nil, errors.Wrap(err, "error to parse filter data value")
			}
			p.NRegPorPagina = v
		case "cOrdenarPor":
			p.COrdenarPor = dv
		case "cOrdemDecrescente":
			p.COrdemDecrescente = dv
		case "nCodTitulo":
			v, err := strconv.Atoi(dv)
			if err != nil {
				return nil, errors.Wrap(err, "error to parse filter data value")
			}
			p.NCodTitulo = v
		case "cCodIntTitulo":
			p.CCodIntTitulo = dv
		case "cNumTitulo":
			p.CNumTitulo = dv
		case "dDtEmisDe":
			p.DDtEmisDe = dv
		case "dDtEmisAte":
			p.DDtEmisAte = dv
		case "dDtVencDe":
			p.DDtVencDe = dv
		case "dDtVencAte":
			p.DDtVencAte = dv
		case "dDtPagtoDe":
			p.DDtPagtoDe = dv
		case "dDtPagtoAte":
			p.DDtPagtoAte = dv
		case "dDtPrevDe":
			p.DDtPrevDe = dv
		case "dDtPrevAte":
			p.DDtPrevAte = dv
		case "dDtRegDe":
			p.DDtRegDe = dv
		case "dDtRegAte":
			p.DDtRegAte = dv
		case "nCodCliente":
			v, err := strconv.Atoi(dv)
			if err != nil {
				return nil, errors.Wrap(err, "error to parse filter data value")
			}
			p.NCodCliente = v
		case "cCPFCNPJCliente":
			p.CCPFCNPJCliente = dv
		case "nCodCtr":
			v, err := strconv.Atoi(dv)
			if err != nil {
				return nil, errors.Wrap(err, "error to parse filter data value")
			}
			p.NCodCtr = v
		case "cNumCtr":
			p.CNumCtr = dv
		case "nCodOS":
			v, err := strconv.Atoi(dv)
			if err != nil {
				return nil, errors.Wrap(err, "error to parse filter data value")
			}
			p.NCodOS = v
		case "cNumOS":
			p.CNumOS = dv
		case "nCodCC":
			v, err := strconv.Atoi(dv)
			if err != nil {
				return nil, errors.Wrap(err, "error to parse filter data value")
			}
			p.NCodCC = v
		case "cStatus":
			p.CStatus = dv
		case "cNatureza":
			p.CNatureza = dv
		case "cTipo":
			p.CTipo = dv
		case "cOperacao":
			p.COperacao = dv
		case "cNumDocFiscal":
			p.CNumDocFiscal = dv
		case "cCodigoBarras":
			p.CCodigoBarras = dv
		case "nCodProjeto":
			v, err := strconv.Atoi(dv)
			if err != nil {
				return nil, errors.Wrap(err, "error to parse filter data value")
			}
			p.NCodProjeto = v
		case "nCodVendedor":
			v, err := strconv.Atoi(dv)
			if err != nil {
				return nil, errors.Wrap(err, "error to parse filter data value")
			}
			p.NCodVendedor = v
		case "nCodComprador":
			v, err := strconv.Atoi(dv)
			if err != nil {
				return nil, errors.Wrap(err, "error to parse filter data value")
			}
			p.NCodComprador = v
		case "cCodCateg":
			p.CCodCateg = dv
		case "dDtIncDe":
			p.DDtIncDe = dv
		case "dDtIncAte":
			p.DDtIncAte = dv
		case "dDtAltDe":
			p.DDtAltDe = dv
		case "dDtAltAte":
			p.DDtAltAte = dv
		case "dDtCancDe":
			p.DDtCancDe = dv
		case "dDtCancAte":
			p.DDtCancAte = dv
		case "cChaveNFe":
			p.CChaveNFe = dv
		}
	}
	r.Param = append(r.Param, p)
	return r, nil
}
